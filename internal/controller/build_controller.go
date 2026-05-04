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
	"regexp"
	"strings"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	ScanImageName     string
}

const insightsBuildPodScannedLabel = "insights.lagoon.sh/scanned"
const insightsScanPodLabel = "insights.lagoon.sh/scan-status"
const insightImageScannerPodLabel = "insights.lagoon.sh/imagescanner"
const dockerHostAnnotationKey = "dockerhost.lagoon.sh/name"
const defaultDockerhost = "docker-host.lagoon.svc"

//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch

// Reconcile is part of the Kubebuilder machinery - it kicks off when we find a build pod in the correct
// state for scanning - i.e. whenever there's a successful build.
func (r *BuildReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("NamespacedName", req.NamespacedName)

	var buildPod corev1.Pod

	if err := r.Get(ctx, req.NamespacedName, &buildPod); err != nil {
		logger.Error(err, fmt.Sprintf("Unable to load Pod- %v", req.Namespace))
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// we check the build pod itself to see if its status is good
	if buildPod.Status.Phase == corev1.PodSucceeded {

		imageList, err := r.scanDeployments(ctx, req, buildPod.Namespace)
		if err != nil {
			logger.Error(err, "Unable to scan deployments")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		dockerhost, err := extractDockerHost(&buildPod)
		if err != nil {
			logger.Info(fmt.Sprintf("Using default dockerhost: %s", err))
			dockerhost = defaultDockerhost
		}

		// now we generate the pod spec we'd like to deploy
		podspec, err := generateScanPodSpec(&buildPod, imageList, r.ScanImageName, dockerhost)
		if err != nil {
			logger.Error(err, "Unable to generate the podspec for the image scanner.")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		// Remove any existing pods
		err = r.killExistingScans(ctx, scannerNameFromBuildname(buildPod.Name), buildPod.Namespace)
		if err != nil {
			logger.Error(err, "Unable to remove existing scan pods")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		// Deploy scan pod
		err = r.Client.Create(ctx, podspec)
		if err != nil {
			logger.Error(err, "Couldn't create pod")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		labels := buildPod.GetLabels()
		// Let's label the pod as having been seen
		labels[insightsBuildPodScannedLabel] = "true" // Right now this assumes a fire and forget style setup
		// TODO: in the future we may want to have a slightly more complex approach to monitoring insights scans

		buildPod.SetLabels(labels)
		err = r.Update(ctx, &buildPod)
		if err != nil {
			logger.Error(err, fmt.Sprintf("Unable to update pod labels: %v", req.NamespacedName.String()))
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	return ctrl.Result{}, nil
}

// extractDockerHost extracts the dockerhost from build pod annotations with fallback to default
func extractDockerHost(buildPod *corev1.Pod) (string, error) {
	if buildPod.Annotations != nil {
		if val, ok := buildPod.Annotations[dockerHostAnnotationKey]; ok {
			if val != "" {
				return val, nil
			}

			return "", errors.New(fmt.Sprintf("Empty %s annotation found on build pod", dockerHostAnnotationKey))
		}
	}

	return "", errors.New(fmt.Sprintf("No %s annotation found on build pod", dockerHostAnnotationKey))
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

// generateScanPodSpec generates the pod spec for the scanner that will be injected into the namespace
func generateScanPodSpec(buildPod *corev1.Pod, images []string, scanImageName, dockerhost string) (*corev1.Pod, error) {

	if len(images) == 0 {
		return nil, errors.New("No images to scan")
	}

	// Define PodSpec

	podSpec := &corev1.Pod{
		ObjectMeta: v12.ObjectMeta{
			Namespace: buildPod.Namespace,
			Name:      scannerNameFromBuildname(buildPod.Name),
			Labels:    imageScanPodLabels(),
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "lagoon-deployer",
			Containers: []corev1.Container{
				{
					Name:            "scanner",
					Image:           scanImageName,
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
			Volumes: []corev1.Volume{
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

	envVars := []corev1.EnvVar{
		{Name: "INSIGHT_SCAN_IMAGES", Value: strings.Join(images, ",")},
		{Name: "NAMESPACE", Value: buildPod.Namespace},
		{Name: "DOCKER_HOST", Value: dockerhost},
	}

	// Copy env vars from the build pod to the scan pod
	copyVars := []string{
		"BRANCH", "BUILD_TYPE", "ENVIRONMENT", "ENVIRONMENT_TYPE", "PROJECT",
		"PR_BASE_BRANCH", "PR_HEAD_BRANCH", "PR_NUMBER",
		"LAGOON_ENVIRONMENT_VARIABLES", "LAGOON_FEATURE_FLAG_.+",
	}
	for i := range buildPod.Spec.Containers[0].Env {
		if containsRegex(copyVars, buildPod.Spec.Containers[0].Env[i].Name) {
			envVars = append(envVars, buildPod.Spec.Containers[0].Env[i])
			continue
		}
	}

	podSpec.Spec.Containers[0].Env = envVars

	return podSpec, nil
}

func (r *BuildReconciler) killExistingScans(ctx context.Context, newScannerName string, namespace string) error {
	// Find pods in namespace with the image scan pod labels
	podlist := &corev1.PodList{}
	ls := client.MatchingLabels(imageScanPodLabels())
	ns := client.InNamespace(namespace)
	err := r.Client.List(ctx, podlist, ns, ls)
	if err != nil {
		log.Log.Info("Issue generating existing scan list")
		return err
	}
	log.Log.Info("Comparing with: " + newScannerName)
	for _, i := range podlist.Items {

		log.Log.Info("Looking at following image by name: " + i.Name)
		//if i.Name != newScannerName { // Then we have a rogue pod
		err = r.Client.Delete(ctx, &i)
		if err != nil {
			return err
		}
		log.Log.Info(fmt.Sprintf("Successfully deleted old insights-scanner pod: %v", i.Name))
		//}
	}
	return nil
}

func imageScanPodLabels() map[string]string {
	return map[string]string{
		insightsScanPodLabel:        "scanning",
		insightImageScannerPodLabel: "true",
	}
}

func scannerNameFromBuildname(buildName string) string {
	return fmt.Sprintf("insights-scanner-%v", buildName)
}

// containsRegex reports whether v matches a regular expression present in s.
func containsRegex(s []string, v string) bool {
	for _, rgx := range s {
		re, err := regexp.Compile(fmt.Sprintf("(?i)^%s$", rgx))
		if err != nil {
			continue
		}

		if re.MatchString(v) {
			return true
		}
	}

	return false
}

// successfulBuildPodsPredicate returns a list of predicate functions to determine
// if the build we're looking at has been successfully completed
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

			_, err = getValueFromMap(labels, insightsScanPodLabel)
			if err == nil {
				return false //this isn't a build pod
			}

			val, err := getValueFromMap(labels, insightsBuildPodScannedLabel)
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
