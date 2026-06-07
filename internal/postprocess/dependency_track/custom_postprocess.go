package deptrack

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	dtrack "github.com/DependencyTrack/client-go"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"lagoon.sh/insights-remote/internal"
	"lagoon.sh/insights-remote/internal/postprocess"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	lagoonEnvDTAPIKey       = "LAGOON_FEATURE_FLAG_INSIGHTS_DEPENDENCY_TRACK_API_KEY"
	lagoonEnvDTPushProdOnly = "LAGOON_FEATURE_FLAG_INSIGHTS_DEPENDENCY_TRACK_PUSH_PROD_ONLY"
)

// resolvePushProdOnly returns the effective pushProdOnly value for a namespace.
// If the secret data contains a valid bool for lagoonEnvDTPushProdOnly, it overrides
// the global default. Invalid values log a warning and fall back to globalDefault.
func resolvePushProdOnly(globalDefault bool, secretData map[string][]byte) bool {
	val, ok := secretData[lagoonEnvDTPushProdOnly]
	if !ok || len(val) == 0 {
		return globalDefault
	}
	parsed, err := strconv.ParseBool(string(val))
	if err != nil {
		slog.Warn("invalid value for push-prod-only override, using global default",
			"value", string(val))
		return globalDefault
	}
	return parsed
}

// CustomPostProcess will send insights for a namespace to a Dependency Track instance configured
// for that namespace.
type CustomPostProcess struct {
	Client       client.Client
	Templates    Templates
	PushProdOnly bool
}

func NewCustomPostProcessor(
	enableDependencyTrackIntegration bool,
	client client.Client,
	pushProdOnly bool,
	dependencyTrackRootProjectNameTemplate string,
	dependencyTrackParentProjectNameTemplate string,
	dependencyTrackProjectNameTemplate string,
	dependencyTrackVersionTemplate string,
) postprocess.PostProcessor {
	if !enableDependencyTrackIntegration {
		return nil
	}

	return &CustomPostProcess{
		Client:       client,
		PushProdOnly: pushProdOnly,
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

	if len(secret.Data[lagoonEnvDTAPIKey]) == 0 {
		return fmt.Errorf("failed to load custom key")
	}

	apiKey := string(secret.Data[lagoonEnvDTAPIKey])
	effectivePushProdOnly := resolvePushProdOnly(d.PushProdOnly, secret.Data)

	client, err := dtrack.NewClient(apiEndpoint, dtrack.WithAPIKey(apiKey))
	if err != nil {
		return err
	}

	err = postProcess(message, effectivePushProdOnly, d.Templates, client)
	return err
}

func (p *CustomPostProcess) Label() string {
	return "dependencytrack.insights.lagoon.sh/custom-status"
}
