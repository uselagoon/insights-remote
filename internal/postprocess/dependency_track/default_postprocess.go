package deptrack

import (
	dtrack "github.com/DependencyTrack/client-go"
	"lagoon.sh/insights-remote/internal"
	"lagoon.sh/insights-remote/internal/postprocess"
)

// DefaultPostProcess will send insights for all namespaces to a central Dependency Track instance.
type DefaultPostProcess struct {
	ApiEndpoint string
	ApiKey      string
	Templates   Templates
}

func NewDefaultPostProcessor(
	enableDependencyTrackIntegration bool,
	dependencyTrackApiEndpoint string,
	dependencyTrackApiKey string,
	dependencyTrackRootProjectNameTemplate string,
	dependencyTrackParentProjectNameTemplate string,
	dependencyTrackProjectNameTemplate string,
	dependencyTrackVersionTemplate string,
) postprocess.PostProcessor {
	if !enableDependencyTrackIntegration {
		return nil
	}

	if dependencyTrackApiEndpoint == "" || dependencyTrackApiKey == "" {
		return nil
	}

	return &DefaultPostProcess{
		ApiEndpoint: dependencyTrackApiEndpoint,
		ApiKey:      dependencyTrackApiKey,
		Templates: newTemplate(
			dependencyTrackRootProjectNameTemplate,
			dependencyTrackParentProjectNameTemplate,
			dependencyTrackProjectNameTemplate,
			dependencyTrackVersionTemplate,
		),
	}
}

func (d *DefaultPostProcess) PostProcess(message internal.LagoonInsightsMessage) error {
	// Only send SBOMs
	if message.Type != internal.InsightsTypeSBOM {
		return nil
	}

	client, err := dtrack.NewClient(d.ApiEndpoint, dtrack.WithAPIKey(d.ApiKey))
	if err != nil {
		return err
	}

	err = postProcess(message, d.Templates, client)
	return err
}
