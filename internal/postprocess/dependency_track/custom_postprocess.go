package deptrack

import (
	"context"
	"fmt"

	dtrack "github.com/DependencyTrack/client-go"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"lagoon.sh/insights-remote/internal"
	"lagoon.sh/insights-remote/internal/postprocess"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CustomPostProcess will send insights for a namespace to a Dependency Track instance configured
// for that namespace.
type CustomPostProcess struct {
	Client    client.Client
	Templates Templates
}

func NewCustomPostProcessor(
	enableDependencyTrackIntegration bool,
	client client.Client,
	dependencyTrackRootProjectNameTemplate string,
	dependencyTrackParentProjectNameTemplate string,
	dependencyTrackProjectNameTemplate string,
	dependencyTrackVersionTemplate string,
) postprocess.PostProcessor {
	if !enableDependencyTrackIntegration {
		return nil
	}

	return &CustomPostProcess{
		Client: client,
		Templates: newTemplate(
			dependencyTrackRootProjectNameTemplate,
			dependencyTrackParentProjectNameTemplate,
			dependencyTrackProjectNameTemplate,
			dependencyTrackVersionTemplate,
		),
	}
}

func (d *CustomPostProcess) PostProcess(message internal.LagoonInsightsMessage) error {
	// Only send SBOMs
	if message.Type != internal.InsightsTypeSBOM {
		return nil
	}

	apiEndpoint := message.Annotations["dependencytrack.insights.lagoon.sh/custom-endpoint"]
	if apiEndpoint == "" {
		return nil
	}

	var secret corev1.Secret
	if err := d.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: message.Namespace,
		Name:      "lagoon-env",
	}, &secret); err != nil {
		return fmt.Errorf("failed to load custom key: %w", err)
	}

	if len(secret.Data["LAGOON_FEATURE_FLAG_INSIGHTS_DEPENDENCY_TRACK_API_KEY"]) == 0 {
		return fmt.Errorf("failed to load custom key")
	}

	apiKey := string(secret.Data["LAGOON_FEATURE_FLAG_INSIGHTS_DEPENDENCY_TRACK_API_KEY"])

	client, err := dtrack.NewClient(apiEndpoint, dtrack.WithAPIKey(apiKey))
	if err != nil {
		return err
	}

	err = postProcess(message, d.Templates, client)
	return err
}
