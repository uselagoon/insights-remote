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
	"k8s.io/apimachinery/pkg/types"
	"strconv"
	"time"

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

const InsightsLabel = "lagoon.sh/insightsType"
const InsightsUpdatedAnnotationLabel = "lagoon.sh/insightsProcessed"
const InsightsWriteDeferred = "lagoon.sh/insightsWriteDeferred"

type LagoonInsightsMessage struct {
	Payload       map[string]string `json:"payload"`
	BinaryPayload map[string][]byte `json:"binaryPayload"`
	Annotations   map[string]string `json:"annotations"`
	Labels        map[string]string `json:"labels"`
	Environment   string            `json:"environment"`
	Project       string            `json:"project"`
	Type          string            `json:"type"`
}

// ConfigMapReconciler reconciles a ConfigMap object
type ConfigMapReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	MessageQWriter   func(data []byte) error
	WriteToQueue     bool
	BurnAfterReading bool
}

//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=configmaps/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ConfigMap object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *ConfigMapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

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
	labels := nameSpace.Labels
	if _, ok := labels["lagoon.sh/environment"]; ok {
		environmentName = labels["lagoon.sh/environment"]
	} else if _, ok := labels["lagoon.sh/project"]; ok {
		projectName = labels["lagoon.sh/project"]
	}

	// insightsType is a way for us to classify incoming insights data, passing
	insightsType := "unclassified"
	if _, ok := labels["insights.lagoon.sh/type"]; ok {
		insightsType = labels["insights.lagoon.sh/type"]
	} else {
		// insightsType can be determined by the incoming data
		if _, ok := labels["lagoon.sh/insightsType"]; ok {
			switch labels["lagoon.sh/insightsType"] {
			case ("sbom-gz"):
				insightsType = "sbom"
			case ("image-gz"):
				insightsType = "inspect"
			}
		}
	}

	// We reject any type that isn't either sbom or inspect to restrict outgoing types
	if insightsType != "sbom" && insightsType != "inspect" {
		//// we mark this configMap as bad, and log an error
		log.Error(nil, fmt.Sprintf("insightsType '%v' unrecognized - rejecting configMap", insightsType))
	} else {
		// Here we attempt to process types using the new name structure

		var sendData = LagoonInsightsMessage{
			Payload:       configMap.Data,
			BinaryPayload: configMap.BinaryData,
			Annotations:   configMap.Annotations,
			Labels:        configMap.Labels,
			Environment:   environmentName,
			Project:       projectName,
			Type:          insightsType,
		}

		marshalledData, err := json.Marshal(sendData)
		if err != nil {
			log.Error(err, "Unable to marshall config data")
			return ctrl.Result{}, err
		}
		err = r.MessageQWriter(marshalledData)

		if err != nil {
			log.Error(err, "Unable to write to message broker")

			//In this case what we want to do is defer the processing to a couple minutes from now
			future := time.Minute * 5
			futureTime := time.Now().Add(future).Unix()
			err = cmlib.LabelCM(ctx, r.Client, configMap, InsightsWriteDeferred, strconv.FormatInt(futureTime, 10))

			if err != nil {
				log.Error(err, "Unable to update configmap")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

	}

	err := cmlib.AnnotateCM(ctx, r.Client, configMap, InsightsUpdatedAnnotationLabel, time.Now().UTC().Format(time.RFC3339))

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
			if labelExists(InsightsLabel, event.Object) &&
				!labelExists(InsightsWriteDeferred, event.Object) &&
				!insightsProcessedAnnotationExists(event.Object) {
				return true
			}
			return false
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			if labelExists(InsightsLabel, event.ObjectNew) &&
				!labelExists(InsightsWriteDeferred, event.ObjectNew) &&
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
	if _, ok := annotations[InsightsUpdatedAnnotationLabel]; ok {
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
