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
	"errors"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"lagoon.sh/insights-remote/internal/tokens"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// NamespaceReconciler reconciles a Namespace object
type NamespaceReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	InsightsJWTSecret string
}

const insightsTokenLabel = "lagoon.sh/insights-token"

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
func (r *NamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var ns corev1.Namespace
	if err := r.Get(ctx, req.NamespacedName, &ns); err != nil {
		log.Error(err, "Unable to load Namespace")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	secretList := &corev1.SecretList{}

	labelSelectorParameters, err := labels.NewRequirement(insightsTokenLabel, selection.Exists, []string{})

	if err != nil {
		log.Error(err, fmt.Sprintf("bad requirement: %v\n\n", err))
	}

	labelSelector := labels.NewSelector()
	labelSelector = labelSelector.Add(*labelSelectorParameters)

	listOptions := client.ListOptions{
		LabelSelector: labelSelector,
		Namespace:     ns.GetName(),
	}
	//r.Get(ctx, "insightsNamespaceToken", secret)
	err = r.Client.List(ctx, secretList, &listOptions)
	if err != nil {
		return ctrl.Result{}, err
	}

	foundItem := false
	deleteSecretMessage := ""
	for _, v := range secretList.Items {
		log.Info(fmt.Sprintf("Found secret with name '%v' and namespace '%v'", v.Name, v.Namespace))
		// let's verify this to make sure it looks good
		if val, ok := v.Data["INSIGHTS_TOKEN"]; ok {
			//log.Info(fmt.Sprintf("Got value of '%v'", string(val)))
			namespaceDetails, err := tokens.ValidateAndExtractNamespaceDetailsFromToken(r.InsightsJWTSecret, string(val))

			if err != nil {
				log.Error(err, "Unable to decode token")
				return ctrl.Result{}, err
			}
			if namespaceDetails.Namespace != ns.GetName() {
				deleteSecretMessage = fmt.Sprintf("Token is invalid - namespaces '%v'!='%v'.", ns.GetName(), namespaceDetails.Namespace)
			}
			foundItem = true
		} else {
			//we delete this secret straight
			deleteSecretMessage = "key INSIGHTS_TOKEN does not exist. Secret is invalid."
		}
		if deleteSecretMessage != "" {
			log.Info(fmt.Sprintf("Removing secret '%v':'%v' - %v", ns.GetName(), v.Name, deleteSecretMessage))
			err = r.Client.Delete(ctx, &v)
			if err != nil {
				log.Error(err, "Unable to delete secret")
				return ctrl.Result{}, err
			}
		}
	}

	if !foundItem { //let's create the token and secret
		labels := ns.GetLabels()
		environmentId, err := getValueFromMap(labels, "lagoon.sh/environmentId")
		if err != nil {
			return ctrl.Result{}, errors.New("Unable to find lagoon.sh/environmentId label in namespace " + ns.GetName() + " failed creating insights token")
		}
		projectName, err := getValueFromMap(labels, "lagoon.sh/project")
		if err != nil {
			return ctrl.Result{}, errors.New("Unable to find lagoon.sh/project label in namespace" + ns.GetName() + " failed creating insights token")
		}
		environmentName, err := getValueFromMap(labels, "lagoon.sh/environment")
		if err != nil {
			return ctrl.Result{}, errors.New("Unable to find lagoon.sh/environment label in namespace " + ns.GetName() + " failed creating insights token")
		}
		jwt, err := tokens.GenerateTokenForNamespace(r.InsightsJWTSecret, tokens.NamespaceDetails{
			Namespace:       ns.GetName(),
			EnvironmentId:   environmentId,
			ProjectName:     projectName,
			EnvironmentName: environmentName,
		})
		if err != nil {
			log.Error(err, "Unable to generate jwt for namespace '%v'", ns.GetName())
		}

		newSecret := corev1.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:      "insights-token",
				Namespace: ns.GetName(),
				Labels: map[string]string{
					insightsTokenLabel: "true",
				},
			},
			Immutable: nil,
			Data: map[string][]byte{
				"INSIGHTS_TOKEN": []byte(jwt),
			},
		}
		err = r.Client.Create(ctx, &newSecret)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func getValueFromMap(labels map[string]string, key string) (string, error) {
	if v, okay := labels[key]; !okay {
		return "", errors.New("key not found")
	} else {
		return v, nil
	}
}

func activeNamespacePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			labels := event.Object.GetLabels()
			_, err := getValueFromMap(labels, "lagoon.sh/environmentId")
			if err != nil {
				return false
			}
			_, err = getValueFromMap(labels, "lagoon.sh/project")
			if err != nil {
				return false
			}
			_, err = getValueFromMap(labels, "lagoon.sh/environment")
			if err != nil {
				return false
			}
			return true
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return false
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			labels := updateEvent.ObjectNew.GetLabels()
			_, err := getValueFromMap(labels, "lagoon.sh/environmentId")
			if err != nil {
				return false
			}
			_, err = getValueFromMap(labels, "lagoon.sh/project")
			if err != nil {
				return false
			}
			_, err = getValueFromMap(labels, "lagoon.sh/environment")
			if err != nil {
				return false
			}
			return true
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return false
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		WithEventFilter(activeNamespacePredicate()).
		Complete(r)
}
