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
	Scheme         *runtime.Scheme
	PostProcessors []postprocess.PostProcessor
}

const minutesBetweenRetries = 5
const maxNumberOfRetries = 2
const postProcRetriesAnnotationKey = "core.insights.lagoon.sh/postproc-retries"
const maxRetryExceededLabelKey = "insights.lagoon.sh/max-retry-exceeded"
const successfulPostProcessRun = "success"
const failedPostProcessRun = "failure"

//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

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
		return ctrl.Result{}, err
	}

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

	// Here we run the post processors
	retryPostprocessors := []string{}
	completePostProcessors := []string{}

	var numberOfRetries int64

	if retriesAnnotation, ok := insightsMessage.Annotations[postProcRetriesAnnotationKey]; ok {
		var err error
		numberOfRetries, err = strconv.ParseInt(retriesAnnotation, 10, 64)
		if err != nil {
			numberOfRetries = 0
			log.Info(fmt.Sprintf("unable to read insights %v label - setting to / assuming first run. Error: %v\n", postProcRetriesAnnotationKey, err.Error()))
		}
	}

	updateAnnotations := map[string]string{}
	for _, processor := range r.PostProcessors {
		if processor != nil {

			// we skip if the processor has been successful in the past
			if val, ok := insightsMessage.Annotations[processor.Label()]; ok {
				if val == successfulPostProcessRun {
					completePostProcessors = append(completePostProcessors, processor.Label())
					continue
				}
			}

			err := processor.PostProcess(insightsMessage)
			if err != nil {
				log.Error(err, "Post processor failed")
				// We line this up for retrying the next time around
				retryPostprocessors = append(retryPostprocessors, processor.Label())
				// let's update our annotations to set this as a failure
				updateAnnotations[processor.Label()] = failedPostProcessRun
				// We'll also want to set or update an annotation telling us why it failed
				updateAnnotations[fmt.Sprintf("%v-error", processor.Label())] = err.Error()
				continue
			}
			completePostProcessors = append(completePostProcessors, processor.Label())
			updateAnnotations[processor.Label()] = successfulPostProcessRun
		}
	}

	updateLabels := map[string]string{}
	if len(retryPostprocessors) > 0 { // There was a problem writing these data to one of the endpoints
		//In this case what we want to do is defer the processing to a couple minutes from now
		updateLabels[internal.InsightsWriteDeferred] = minutesFromNow(minutesBetweenRetries)
		numberOfRetries += 1

		if numberOfRetries > maxNumberOfRetries { // We have now reached the end of the line.
			updateLabels[maxRetryExceededLabelKey] = "MAX_RETRY_EXCEEDED"
			log.Info(fmt.Sprintf("Max post-processor retries (%v) exceeded for configmap %v/%v - no further retries will be attempted", maxNumberOfRetries, configMap.Namespace, configMap.Name))
		}

	} else { // we've managed to get everything stowed away, let's mark this as done.
		updateAnnotations[internal.InsightsUpdatedAnnotationLabel] = time.Now().UTC().Format(time.RFC3339)
	}

	// let's write the number of retries either way
	updateAnnotations[postProcRetriesAnnotationKey] = strconv.FormatInt(numberOfRetries, 10)

	err := cmlib.BatchUpdateCM(ctx, r.Client, configMap, updateLabels, updateAnnotations)

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

// Let's set up a predicate that filters out anything without a particular label AND
// we don't care about delete events
func insightMaxRetryPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			if labelExists(maxRetryExceededLabelKey, event.Object) {
				return false
			}
			return true
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			if labelExists(maxRetryExceededLabelKey, event.ObjectNew) {
				return false
			}
			return true
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
		WithEventFilter(insightMaxRetryPredicate()).
		Complete(r)
}

func minutesFromNow(mins int) string {
	xMins := time.Minute * time.Duration(mins)
	return strconv.FormatInt(time.Now().Add(xMins).Unix(), 10)
}
