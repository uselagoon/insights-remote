package controllers

import (
	"context"
	"fmt"
	"github.com/aquasecurity/trivy-operator/pkg/apis/aquasecurity/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strconv"
)

const LagoonProjectLabel = "lagoon.sh/project"
const TrivyVulnReportNamespace = "trivy-operator.resource.namespace"

type TrivyVulnerabilityReportReconciler struct {
	Client         client.Client
	Scheme         *runtime.Scheme
	MessageQWriter func(data []byte) error
	WriteToQueue   bool
	PredicateData  interface{}
}

type ProblemsPayload struct {
	Problems []interface{} `json:"problems"`
}

// predicate that filters out non-lagoon projects
func (r *TrivyVulnerabilityReportReconciler) lagoonProjectLabelsOnlyPredicate() predicate.Predicate {

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			if r.isLagoonProject(event.Object) &&
				!insightsProcessedAnnotationExists(event.Object) {
				return true
			}
			return false
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			if r.isLagoonProject(event.ObjectNew) &&
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

func (r *TrivyVulnerabilityReportReconciler) isLagoonProject(event client.Object) bool {
	var namespace string
	for k, v := range event.GetLabels() {
		if k == TrivyVulnReportNamespace {
			namespace = v
		}
	}

	nsObj := &corev1.Namespace{}
	nsObj.SetName(namespace)

	// retrieve the ns object from the k8 API
	err := r.Client.Get(context.Background(), client.ObjectKeyFromObject(nsObj), nsObj)
	if err != nil {
		fmt.Printf("Failed to get Namespace %q: %v\n", namespace, err)
		os.Exit(1)
	}

	// check the ns if this is a lagoon project
	isLagoonProject := false
	for k, v := range nsObj.Labels {
		if k == LagoonProjectLabel && v == namespace {
			isLagoonProject = true
		}
	}

	if isLagoonProject {
		// store some data since we've already done the lookup, and it doesn't exist in the vuln. report
		r.PredicateData = nsObj.Labels

		return true
	}

	return false
}

func (r *TrivyVulnerabilityReportReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	vulnReports := &v1alpha1.VulnerabilityReportList{}
	err := r.Client.List(ctx, vulnReports)
	if err != nil {
		return reconcile.Result{}, err
	}

	for _, vulnReport := range vulnReports.Items {
		if vulnReport.Namespace == request.Namespace && vulnReport.Name == request.Name {
			log := log.FromContext(ctx)

			bytes, err := json.Marshal(vulnReport)
			if err != nil {
				return reconcile.Result{}, err
			}

			var report v1alpha1.VulnerabilityReport
			if err := json.Unmarshal(bytes, &report); err != nil {
				return reconcile.Result{}, err
			}

			labelsWithProjectMeta := r.appendProjectMetaToReportLabels(report)

			// take report and convert to lagoon problems payload object
			problemsPayload := updateReportPayloadToLagoonProblems(report)

			var sendData = LagoonInsightsMessage{
				Payload:     []interface{}{problemsPayload},
				Annotations: report.Annotations,
				Labels:      labelsWithProjectMeta,
			}

			// marshall data ready for sending to broker
			marshalledData, err := json.Marshal(sendData)
			if err != nil {
				log.Error(err, "Unanle to marshall Vulnerability report data")
				return ctrl.Result{}, err
			}

			//log.Info(fmt.Sprintf("VulnerabilityReport CRD name: %s", marshalledData))
			//log.Info(fmt.Sprintf("VulnerabilityReport CRD: %s\n", bytes))
			//log.Info(fmt.Sprintf("VulnerabilityReport CRD: %s\n", report.ObjectMeta.Name))

			if r.WriteToQueue {
				if err := r.MessageQWriter(marshalledData); err != nil {

					log.Error(err, "Unable to write to message broker")

					return ctrl.Result{}, err
				}
			}

			// add an annotation to the report indicating it has been processed
			report.Annotations["lagoon.sh/insightsProcessed"] = "true"
			if err := r.Client.Update(ctx, &report); err != nil {
				return ctrl.Result{}, err
			}

		}
	}

	return reconcile.Result{}, nil
}

func updateReportPayloadToLagoonProblems(report v1alpha1.VulnerabilityReport) interface{} {
	var problems []map[string]interface{}
	labels := report.GetLabels()

	for _, problem := range report.Report.Vulnerabilities {
		var score float64
		if problem.Score != nil {
			score = *problem.Score
		}
		scoreStr := strconv.FormatFloat(score, 'f', 1, 64)
		s, _ := strconv.ParseFloat(scoreStr, 64)
		scoreFloat := s * 0.1

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
			"severityScore": fmt.Sprintf("%.2f", scoreFloat),
		}

		problems = append(problems, p)
	}

	return map[string]interface{}{"problems": problems}
}

func (r *TrivyVulnerabilityReportReconciler) appendProjectMetaToReportLabels(report v1alpha1.VulnerabilityReport) map[string]string {
	l := report.Labels
	m := report.ObjectMeta

	if labels, ok := r.PredicateData.(map[string]string); ok {
		for k, v := range labels {
			l[k] = v
		}
	}

	l["lagoon.sh/service"] = m.Labels["trivy-operator.container.name"]
	l["lagoon.sh/insightsType"] = "trivy-vuln-report"

	return l
}

func (r *TrivyVulnerabilityReportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.VulnerabilityReport{}).
		WithEventFilter(r.lagoonProjectLabelsOnlyPredicate()).
		Complete(r)
}