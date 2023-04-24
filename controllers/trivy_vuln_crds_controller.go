package controllers

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/aquasecurity/trivy-operator/pkg/apis/aquasecurity/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	LagoonProjectLabel        = "lagoon.sh/project"
	TrivyVulnReportNamespace  = "trivy-operator.resource.namespace"
	LagoonInsightsProcessed   = "lagoon.sh/insightsProcessed"
	ProcessingTimeoutDuration = 5 * time.Second
)

type TrivyVulnerabilityReportReconciler struct {
	Client           client.Client
	Scheme           *runtime.Scheme
	MessageQWriter   func(data []byte) error
	WriteToQueue     bool
	PredicateDataMap sync.Map
}

type ProblemsPayload struct {
	Problems []interface{} `json:"problems"`
}

// predicate that filters out non-lagoon projects
func (r *TrivyVulnerabilityReportReconciler) lagoonProjectLabelsOnlyPredicate() predicate.Predicate {

	filterFunc := func(obj client.Object) bool {
		return isLagoonProject(r, obj) && !insightsProcessedAnnotationExists(obj)
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return filterFunc(event.Object)
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return filterFunc(event.ObjectNew)
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return false
		},
	}
}

func isLagoonProject(r *TrivyVulnerabilityReportReconciler, event client.Object) bool {
	namespace := event.GetLabels()[TrivyVulnReportNamespace]
	nsObj := &corev1.Namespace{}
	nsObj.SetName(namespace)

	// retrieve the ns object from the k8 API
	if err := r.Client.Get(context.Background(), client.ObjectKeyFromObject(nsObj), nsObj); err != nil {
		fmt.Printf("Failed to get Namespace %q: %v\n", namespace, err)
		os.Exit(1)
	}

	// check if the ns is a lagoon project
	if val, ok := nsObj.Labels[LagoonProjectLabel]; ok && val == namespace {
		// store some data since we've already done the lookup, and it doesn't exist in the vuln. report
		r.PredicateDataMap.Store(client.ObjectKeyFromObject(event), nsObj.Labels)
		return true
	}

	return false
}

func (r *TrivyVulnerabilityReportReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	// Retrieve the VulnerabilityReport object
	vulnReport := &v1alpha1.VulnerabilityReport{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: request.Name, Namespace: request.Namespace}, vulnReport)
	if err != nil {
		return reconcile.Result{}, err
	}

	log := log.FromContext(ctx)

	// Copy the object
	report := vulnReport.DeepCopy()

	// Append project metadata to the report labels
	labelsWithProjectMeta := r.appendProjectMetaToReportLabels(report, request)

	// Convert the report to a Lagoon problems payload object
	problemsPayload := updateReportPayloadToLagoonProblems(report)

	// Construct the message to be sent to the broker
	sendData := LagoonInsightsMessage{
		Payload:     []interface{}{problemsPayload},
		Annotations: report.Annotations,
		Labels:      labelsWithProjectMeta,
	}

	// Marshal the message data
	marshalledData, err := json.Marshal(sendData)
	if err != nil {
		log.Error(err, "Unable to marshall Vulnerability report data")
		return ctrl.Result{}, err
	}

	// Write the message to the broker if enabled
	if r.WriteToQueue {
		if err := r.MessageQWriter(marshalledData); err != nil {
			log.Error(err, "Unable to write to message broker")
			return ctrl.Result{}, err
		}
	}

	// Add an annotation to the report indicating it has been processed
	report.Annotations["lagoon.sh/insightsProcessed"] = "true"
	if err := r.Client.Update(ctx, report); err != nil {
		return ctrl.Result{}, err
	}

	return reconcile.Result{}, nil
}

func updateReportPayloadToLagoonProblems(report *v1alpha1.VulnerabilityReport) interface{} {
	problems := make([]map[string]interface{}, 0, len(report.Report.Vulnerabilities))

	labels := report.GetLabels()
	for _, problem := range report.Report.Vulnerabilities {
		score := 0.0
		if problem.Score != nil {
			score = *problem.Score
		}
		scoreStr := strconv.FormatFloat(score*0.1, 'f', 2, 64)

		p := map[string]interface{}{
			"environment":       labels["lagoon.sh/environmentId"],
			"identifier":        problem.VulnerabilityID,
			"associatedPackage": problem.Resource,
			"version":           problem.InstalledVersion,
			"fixedVersion":      problem.FixedVersion,
			"source":            report.Report.Scanner.Name,
			"service":           report.Report.Artifact.Repository,
			"data":              "{}",
			"severity":          problem.Severity,
			"description": func() string {
				if problem.Description == "" {
					return problem.Title
				}
				return problem.Description
			}(),
			"links":         problem.PrimaryLink,
			"severityScore": scoreStr,
		}

		problems = append(problems, p)
	}

	return map[string]interface{}{
		"problems": problems,
	}
}

func (r *TrivyVulnerabilityReportReconciler) appendProjectMetaToReportLabels(report *v1alpha1.VulnerabilityReport, request reconcile.Request) map[string]string {
	labels := make(map[string]string, len(report.Labels)+2)
	for k, v := range report.Labels {
		labels[k] = v
	}
	labels["lagoon.sh/service"] = report.Labels["trivy-operator.container.name"]
	labels["lagoon.sh/insightsType"] = "trivy-vuln-report"

	// Retrieve the PredicateData for this reconciliation
	if predicateData, ok := r.PredicateDataMap.Load(client.ObjectKey{Name: request.Name, Namespace: request.Namespace}); ok {
		if predLabels, ok := predicateData.(map[string]string); ok {
			for k, v := range predLabels {
				labels[k] = v
			}
		}
	}

	return labels
}

func (r *TrivyVulnerabilityReportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.VulnerabilityReport{}).
		WithEventFilter(r.lagoonProjectLabelsOnlyPredicate()).
		Complete(r)
}
