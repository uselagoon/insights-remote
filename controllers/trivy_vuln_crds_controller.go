package controllers

import (
	"context"
	"fmt"
	"github.com/aquasecurity/trivy-operator/pkg/apis/aquasecurity/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strings"
)

type TrivyVulnerabilityReportReconciler struct {
	Client         client.Client
	Scheme         *runtime.Scheme
	MessageQWriter func(data []byte) error
	WriteToQueue   bool
}

func (r *TrivyVulnerabilityReportReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	// define the label selector to matches the VulnerabilityReports we are interested in which are Lagoon apps.
	labelSelector := labels.SelectorFromSet(map[string]string{"lagoon.sh": "true"})

	// get the list of Trivy VulnerabilityReport CRDs that match the given label selector
	vulnReports := &v1alpha1.VulnerabilityReportList{}
	err := r.Client.List(ctx, vulnReports, &client.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return reconcile.Result{}, err
	}

	// Log the JSON representation of each VulnerabilityReport CRD
	for _, vulnReport := range vulnReports.Items {
		log := log.FromContext(ctx)

		bytes, err := json.Marshal(vulnReport)
		if err != nil {
			return reconcile.Result{}, err
		}

		var report v1alpha1.VulnerabilityReport
		if err := json.Unmarshal(bytes, &report); err != nil {
			return reconcile.Result{}, err
		}

		// extract project/environment related meta from report and add to existing labels so lagoon knows where to push these insights
		labelsWithProjectMeta := appendProjectMetaToReportLabels(report.Labels, report.ObjectMeta)

		var sendData = LagoonInsightsMessage{
			Payload:     report.Report,
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

		log.Info(fmt.Sprintf("VulnerabilityReport CRD: %s\n", report.ObjectMeta.Name))

		if r.WriteToQueue {
			if err := r.MessageQWriter(marshalledData); err != nil {

				log.Error(err, "Unable to write to message broker")

				//In this case what we want to do is defer the processing to a couple of minutes from now
				//future := time.Minute * 5
				//futureTime := time.Now().Add(future).Unix()
				//err = cmlib.LabelCM(ctx, r.Client, configMap, InsightsWriteDeferred, strconv.FormatInt(futureTime, 10))
				//
				//if err != nil {
				//	log.Error(err, "Unable to update report")
				//	return ctrl.Result{}, err
				//}

				return ctrl.Result{}, err
			}
		}

	}

	return reconcile.Result{}, nil
}

func appendProjectMetaToReportLabels(l map[string]string, meta v1.ObjectMeta) map[string]string {
	// split namespace into project and environment
	index := strings.LastIndex(meta.Namespace, "-")
	project := meta.Namespace
	environment := ""
	if index == -1 {
	} else {
		project = meta.Namespace[:index]
		environment = meta.Namespace[index+1:]
	}

	l["lagoon.sh/project"] = project
	l["lagoon.sh/environment"] = environment
	l["lagoon.sh/service"] = meta.Labels["trivy-operator.container.name"]
	l["lagoon.sh/insightsType"] = "trivy-vuln-report"

	return l
}

func (r *TrivyVulnerabilityReportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.VulnerabilityReport{}).
		Complete(r)
}
