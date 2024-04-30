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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// BuildReconciler reconciles a Build Pod object
type BuildReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	InsightsJWTSecret string
}

const insightsScannedLabel = "insights.lagoon.sh/scanned"

//+kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=namespaces/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=namespaces/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Namespace object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *BuildReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var buildPod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &buildPod); err != nil {
		log.Error(err, "Unable to load Pod")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// we check the build pod itself to see if its status is good
	if buildPod.Status.Phase == corev1.PodSucceeded {
		labels := buildPod.GetLabels()
		// Let's label the pod as having been seen
		labels[insightsScannedLabel] = "true"
		buildPod.SetLabels(labels)
		err := r.Update(ctx, &buildPod)
		if err != nil {
			log.Error(err, fmt.Sprintf("Unable to update pod labels: %v", req.NamespacedName.String()))
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	return ctrl.Result{}, nil
}

func successfulBuildPodsPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool { return false },
		DeleteFunc: func(event event.DeleteEvent) bool { return false },
		UpdateFunc: func(event event.UpdateEvent) bool {

			//TODO: need the logic here to find the appropriate types
			// that is, successful and build pods

			labels := event.ObjectNew.GetLabels()
			_, err := getValueFromMap(labels, "lagoon.sh/buildName")
			if err != nil {
				return false //this isn't a build pod
			}

			val, err := getValueFromMap(labels, insightsScannedLabel)
			if err == nil && val == "false" {
				return false
			}
			return true

		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return false
		},
	}
}

//
//func activeNamespacePredicate(tokenTargetLabel string) predicate.Predicate {
//	return predicate.Funcs{
//		CreateFunc: func(event event.CreateEvent) bool {
//			labels := event.Object.GetLabels()
//			_, err := getValueFromMap(labels, "lagoon.sh/environmentId")
//			if err != nil {
//				return false
//			}
//			_, err = getValueFromMap(labels, "lagoon.sh/project")
//			if err != nil {
//				return false
//			}
//			_, err = getValueFromMap(labels, "lagoon.sh/environment")
//			if err != nil {
//				return false
//			}
//
//			if tokenTargetLabel != "" {
//				_, err = getValueFromMap(labels, tokenTargetLabel)
//				if err != nil {
//					return false
//				}
//			}
//
//			return true
//		},
//		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
//			return false
//		},
//		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
//			labels := updateEvent.ObjectNew.GetLabels()
//			_, err := getValueFromMap(labels, "lagoon.sh/environmentId")
//			if err != nil {
//				return false
//			}
//			_, err = getValueFromMap(labels, "lagoon.sh/project")
//			if err != nil {
//				return false
//			}
//			_, err = getValueFromMap(labels, "lagoon.sh/environment")
//			if err != nil {
//				return false
//			}
//			if tokenTargetLabel != "" {
//				_, err = getValueFromMap(labels, tokenTargetLabel)
//				if err != nil {
//					return false
//				}
//			}
//			return true
//		},
//		GenericFunc: func(genericEvent event.GenericEvent) bool {
//			return false
//		},
//	}
//}

// SetupWithManager sets up the controller with the Manager.
func (r *BuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(successfulBuildPodsPredicate()).
		Complete(r)
}
