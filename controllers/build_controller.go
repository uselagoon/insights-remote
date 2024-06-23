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
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"strings"
)

// BuildReconciler reconciles a Build Pod object
type BuildReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	InsightsJWTSecret string
	//ScanImageName     string // TODO: make this something that's passed as an argument
}

const insightsScannedLabel = "insights.lagoon.sh/scanned"
const scanImageName = "imagecache.amazeeio.cloud/bomoko/insights-scan:latest"

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

		imageList, err := r.scanDeployments(ctx, req, buildPod.Namespace)
		if err != nil {
			log.Error(err, "Unable to scan deployments")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		// now we generate the pod spec we'd like to deploy
		podspec, err := generateScanPodSpec(imageList, buildPod.Name, buildPod.Namespace)
		if err != nil {
			log.Error(err, "Unable to generate podspec")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		// Remove any existing pods
		err = r.killExistingScans(ctx, scannerNameFromBuildname(buildPod.Name), buildPod.Namespace)
		if err != nil {
			log.Error(err, "Unable to remove existing scan pods")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		// Deploy scan pod
		err = r.Client.Create(ctx, podspec)
		if err != nil {
			log.Error(err, "Couldn't create pod")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		labels := buildPod.GetLabels()
		// Let's label the pod as having been seen
		labels[insightsScannedLabel] = "true"
		buildPod.SetLabels(labels)
		err = r.Update(ctx, &buildPod)
		if err != nil {
			log.Error(err, fmt.Sprintf("Unable to update pod labels: %v", req.NamespacedName.String()))
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	return ctrl.Result{}, nil
}

// scanDeployments will look at all the deployments in a namespace and
// if they're labeled correctly, bundle their images into a single scan
func (r *BuildReconciler) scanDeployments(ctx context.Context, req ctrl.Request, namespace string) ([]string, error) {

	deploymentList := &v1.DeploymentList{}
	err := r.List(ctx, deploymentList, client.InNamespace(namespace))
	if err != nil {
		return []string{}, err
	}
	var imageList []string
	for _, d := range deploymentList.Items {
		log.Log.Info(fmt.Sprintf("Found deployment '%v' in namespace '%v'\n", d.Name, namespace))

		// TODO so we want to filter deployments based on the appropriate labels

		// Let's get a list of all the images involved
		for _, i := range d.Spec.Template.Spec.Containers {
			imageList = append(imageList, i.Image)
			log.Log.Info(fmt.Sprintf("    Found image: %v\n", i.Image))
		}
	}

	return imageList, nil
}

func generateScanPodSpec(images []string, buildName string, namespace string) (*corev1.Pod, error) {

	if len(images) == 0 {
		return nil, errors.New("No images to scan")
	}

	insightScanImages := strings.Join(images, ",")

	// Define PodSpec

	podSpec := &corev1.Pod{
		ObjectMeta: v12.ObjectMeta{
			Namespace: namespace,
			Name:      scannerNameFromBuildname(buildName),
			Labels:    imageScanPodLabels(),
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "lagoon-deployer",
			Containers: []corev1.Container{
				{
					Name:  "scanner",
					Image: scanImageName,
					Env: []corev1.EnvVar{
						{
							Name:  "INSIGHT_SCAN_IMAGES",
							Value: insightScanImages,
						},
						{
							Name:  "NAMESPACE",
							Value: namespace,
						},
					},
					ImagePullPolicy: "Always",
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "lagoon-internal-registry-secret",
							MountPath: "/home/.docker/",
							ReadOnly:  true,
						},
					},
				},
			},
			RestartPolicy: "Never",
			Volumes: []corev1.Volume{ // Here we have to mount the
				{
					Name: "lagoon-internal-registry-secret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "lagoon-internal-registry-secret",
							Items: []corev1.KeyToPath{
								{Key: ".dockerconfigjson", Path: "config.json"},
							},
						},
					},
				},
			},
		},
	}
	return podSpec, nil
}

func (r *BuildReconciler) killExistingScans(ctx context.Context, newScannerName string, namespace string) error {
	// Find pods in namespace with the image scan pod labels

	podlist := &corev1.PodList{}
	//ls := client.MatchingLabels{"insights.lagoon.sh/imagescanner": "scanning"}
	ls := client.MatchingLabels(imageScanPodLabels())
	ns := client.InNamespace(namespace)
	err := r.Client.List(ctx, podlist, ns, ls)
	if err != nil {
		return err
	}
	for _, i := range podlist.Items {
		if i.Name != newScannerName { // Then we have a rogue pod
			//r.Client.Delete(ctx, &i)
			log.Log.Info(fmt.Sprintf("Going to delete pod: %v", i.Name))
		}
	}
	return nil
}

func imageScanPodLabels() map[string]string {
	return map[string]string{
		"insights.lagoon.sh/imagescanner": "scanning",
		"lagoon.sh/buildName":             "notabuild",
	}
}

func scannerNameFromBuildname(buildName string) string {
	return fmt.Sprintf("insights-scanner-%v", buildName)
}

func successfulBuildPodsPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool { return false },
		DeleteFunc: func(event event.DeleteEvent) bool { return false },
		UpdateFunc: func(event event.UpdateEvent) bool {

			//TODO: need the logic here to find the appropriate types
			// that is, successful and build pods

			// TODO: remove when happy with process - and we want it to run across everything
			if event.ObjectNew.GetNamespace() != "test6-drupal-example-simple-test1copy" {
				return false
			}

			labels := event.ObjectNew.GetLabels()
			_, err := getValueFromMap(labels, "lagoon.sh/buildName")
			if err != nil {
				log.Log.Info(fmt.Sprintf("Not a build pod, skipping : %v", event.ObjectNew.GetName()))
				return false //this isn't a build pod
			}

			_, err = getValueFromMap(labels, "insights.lagoon.sh/imagescanner")
			if err == nil {
				log.Log.Info(fmt.Sprintf("Found a scanner pod, skipping : %v", event.ObjectNew.GetName()))
				return false //this isn't a build pod
			}

			val, err := getValueFromMap(labels, insightsScannedLabel)
			if err == nil {
				log.Log.Info(fmt.Sprintf("Build pod already scanned, skipping : %v - value: %v", event.ObjectNew.GetName(), val))
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
func (r *BuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(successfulBuildPodsPredicate()).
		Complete(r)
}
