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
	"github.com/cheshir/go-mq"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const insightsLabel = "lagoon.sh/insightsType"
const insightsUpdatedAnnotationLabel = "lagoon.sh/insightsProcessed"

type LagoonInsightsMessage struct {
	Payload       map[string]string `json:"payload"`
	BinaryPayload map[string][]byte `json:"binaryPayload"`
	Annotations   map[string]string `json:"annotations"`
	Labels        map[string]string `json:"labels"`
}

// ConfigMapReconciler reconciles a ConfigMap object
type ConfigMapReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	MessageQ mq.MQ
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

	producer, err := r.MessageQ.SyncProducer("lagoon-insights")
	if err != nil {
		log.Error(err, "Unable to write to message broker")
		return ctrl.Result{}, err
	}

	var sendData = LagoonInsightsMessage{
		Payload:       configMap.Data,
		BinaryPayload: configMap.BinaryData,
		Annotations:   configMap.Annotations,
		Labels:        configMap.Labels,
	}

	marshalledData, err := json.Marshal(sendData)
	if err != nil {
		log.Error(err, "Unable to marshall config data")
		return ctrl.Result{}, err
	}

	err = producer.Produce(marshalledData)

	if err != nil {
		log.Error(err, "Unable to write to message broker")
		return ctrl.Result{}, err
	}

	//write it to the message broker ...
	//log.Info(configMap.Data["syftoutput"])

	annotations := configMap.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[insightsUpdatedAnnotationLabel] = time.Now().UTC().String()
	configMap.SetAnnotations(annotations)

	if err := r.Update(ctx, &configMap); err != nil {
		log.Error(err, "Unable to update configMap")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// Let's set up a predicate that filters out anything without a particular label AND
// we don't care about delete events
func insightLabelsOnlyPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			for k, v := range event.Object.GetLabels() {
				if (k == insightsLabel || v == insightsLabel) && insightsProcessedAnnotationExists(event.Object) != true {
					//fmt.Println("Got one that should be processed " + event.Object.GetName())
					return true
				}
			}
			return false
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			for k, v := range event.ObjectNew.GetLabels() {
				if (k == insightsLabel || v == insightsLabel) && insightsProcessedAnnotationExists(event.ObjectNew) != true {
					return true
				}
			}
			return false
		},
	}
}

func insightsProcessedAnnotationExists(eventObject client.Object) bool {
	annotations := eventObject.GetAnnotations()
	annotationExists := false
	if _, ok := annotations[insightsUpdatedAnnotationLabel]; ok {
		fmt.Println("Insights update annotation exists for " + eventObject.GetName())
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
