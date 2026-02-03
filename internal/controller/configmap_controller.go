/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"lagoon.sh/insights-remote/internal"
	"lagoon.sh/insights-remote/internal/postprocess"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"lagoon.sh/insights-remote/cmlib"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ConfigMapReconciler reconciles a ConfigMap object
type ConfigMapReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	MessageQWriter   func(data []byte) error
	WriteToQueue     bool
	BurnAfterReading bool
	PostProcessors   postprocess.PostProcessors
}

//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=configmaps/finalizers,verbs=update

// Gathers insights related Config Maps and ships them to configured endpoints.
func (r *ConfigMapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var insightsMessage internal.LagoonInsightsMessage

	var configMap corev1.ConfigMap
	if err := r.Get(ctx, req.NamespacedName, &configMap); err != nil {
		log.Error(err, "Unable to load configMap")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var nameSpace corev1.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: req.Namespace}, &nameSpace); err != nil {
		log.Error(err, "Unable to load Namespace")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var environmentName string
	var projectName string
	var serviceName string
	labels := nameSpace.Labels
	if _, ok := labels["lagoon.sh/environment"]; ok {
		environmentName = labels["lagoon.sh/environment"]
	}
	if _, ok := labels["lagoon.sh/project"]; ok {
		projectName = labels["lagoon.sh/project"]
	}

	if _, ok := configMap.Labels["lagoon.sh/service"]; ok {
		serviceName = configMap.Labels["lagoon.sh/service"]
	}
	// insightsType is a way for us to classify incoming insights data, passing
	insightsType := internal.InsightsTypeUnclassified

	if _, ok := configMap.Labels["insights.lagoon.sh/type"]; ok {
		insightsType = configMap.Labels["insights.lagoon.sh/type"]
		log.Info(fmt.Sprintf("Found insights.lagoon.sh/type:%v", insightsType))
	} else {
		// insightsType can be determined by the incoming data
		if _, ok := configMap.Labels["lagoon.sh/insightsType"]; ok {
			switch configMap.Labels["lagoon.sh/insightsType"] {
			case "sbom-gz":
				log.Info("Inferring insights type of sbom")
				insightsType = internal.InsightsTypeSBOM
			case "image-gz":
				log.Info("Inferring insights type of inspect")
				insightsType = internal.InsightsTypeInspect
			}
		}
	}

	// We reject any type that isn't either sbom or inspect to restrict outgoing types
	if insightsType != internal.InsightsTypeSBOM && insightsType != internal.InsightsTypeInspect {
		//// we mark this configMap as bad, and log an error
		err := fmt.Errorf("insightsType '%v' unrecognized - rejecting configMap", insightsType)
		log.Error(err, err.Error())
	} else {
		// Here we attempt to process types using the new name structure

		insightsMessage = internal.LagoonInsightsMessage{
			Payload:       configMap.Data,
			BinaryPayload: configMap.BinaryData,
			Annotations:   configMap.Annotations,
			Labels:        configMap.Labels,
			Namespace:     configMap.Namespace,
			Environment:   environmentName,
			Project:       projectName,
			Type:          insightsType,
			Service:       serviceName,
		}

		marshalledData, err := json.Marshal(insightsMessage)
		if err != nil {
			log.Error(err, "Unable to marshall config data")
			return ctrl.Result{}, err
		}

		err = r.MessageQWriter(marshalledData)

		if err != nil {
			log.Error(err, "Unable to write to message broker")

			//In this case what we want to do is defer the processing to a couple minutes from now
			err = cmlib.LabelCM(ctx, r.Client, configMap, internal.InsightsWriteDeferred, minutesFromNow(5))

			if err != nil {
				log.Error(err, "Unable to update configmap")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

	}

	// Here we run the post processors
	retryPostprocessing := false
	for _, processor := range r.PostProcessors.PostProcessors {
		err := processor.PostProcess(insightsMessage)
		if err != nil {
			log.Error(err, "Post processor failed")
			retryPostprocessing = true
		}
	}

	if retryPostprocessing {
		//In this case what we want to do is defer the processing to a couple minutes from now
		err := cmlib.LabelCM(ctx, r.Client, configMap, internal.InsightsWriteDeferred, minutesFromNow(5))

		if err != nil {
			log.Error(err, "Unable to update configmap")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	}

	err := cmlib.AnnotateCM(ctx, r.Client, configMap, internal.InsightsUpdatedAnnotationLabel, time.Now().UTC().Format(time.RFC3339))

	if err != nil {
		log.Error(err, "Unable to update configmap")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// Let's set up a predicate that filters out anything without a particular label AND
// we don't care about delete events
func insightLabelsOnlyPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			if labelExists(internal.InsightsLabel, event.Object) &&
				!labelExists(internal.InsightsWriteDeferred, event.Object) &&
				!insightsProcessedAnnotationExists(event.Object) {
				return true
			}
			return false
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			if labelExists(internal.InsightsLabel, event.ObjectNew) &&
				!labelExists(internal.InsightsWriteDeferred, event.ObjectNew) &&
				!insightsProcessedAnnotationExists(event.ObjectNew) {
				return true
			}
			return false
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return false
		},
	}
}

func labelExists(label string, event client.Object) bool {
	for k, v := range event.GetLabels() {
		if k == label || v == label {
			return true
		}
	}
	return false
}

func insightsProcessedAnnotationExists(eventObject client.Object) bool {
	annotations := eventObject.GetAnnotations()
	annotationExists := false
	if _, ok := annotations[internal.InsightsUpdatedAnnotationLabel]; ok {
		log.Log.Info(fmt.Sprintf("Insights update annotation exists for '%v' in ns '%v'", eventObject.GetName(), eventObject.GetNamespace()))
		annotationExists = true
	}
	return annotationExists
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConfigMapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithEventFilter(insightLabelsOnlyPredicate()).
		Complete(r)
}

func minutesFromNow(mins int) string {
	xMins := time.Minute * time.Duration(mins)
	return strconv.FormatInt(time.Now().Add(xMins).Unix(), 10)
}
