package controllers

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"lagoon.sh/insights-remote/service"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	lagoonv1beta1 "github.com/uselagoon/remote-controller/apis/lagoon/v1beta1"
)

type LagoonBuildReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Manager          manager.Manager
	MessageQWriter   func(data []byte) error
	WriteToQueue     bool
	BurnAfterReading bool
}

func (r *LagoonBuildReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var lagoonBuild lagoonv1beta1.LagoonBuild
	if err := r.Get(ctx, req.NamespacedName, &lagoonBuild); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "Unable to load LagoonBuild")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	namespace := lagoonBuild.Namespace
	buildName := lagoonBuild.Name
	prroject := lagoonBuild.Spec.Project.Name
	environment := strings.Split(lagoonBuild.Spec.Project.Environment, "Name")[0]

	images, err := service.RunGetImageList(r.Manager, namespace)

	// // Run image inspection
	// imageInspectOutputFile := fmt.Sprintf("%s/%s.image-inspect.json.gz", "/tmp", imageName)
	// err := service.RunImageInspect(imageFull, imageInspectOutputFile)
	// if err != nil {
	// 	log.Error(err, "Failed to run image inspection:")
	// }

	// Run SBOM scan on images
	err = service.RunSbomScan(images, namespace, buildName, prroject, environment)
	if err != nil {
		log.Error(err, "Failed to run SBOM scan:")
	}

	fmt.Println("Service execution completed successfully")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LagoonBuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Assign the manager to the reconciler
	r.Manager = mgr

	return ctrl.NewControllerManagedBy(mgr).
		For(&lagoonv1beta1.LagoonBuild{}).
		// WithEventFilter(insightLabelsOnlyPredicate()).
		Complete(r)
}
