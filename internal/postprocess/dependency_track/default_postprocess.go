package deptrack

import (
	dtrack "github.com/DependencyTrack/client-go"
	"lagoon.sh/insights-remote/internal"
)

// DefaultPostProcess will send insights for all namespaces to a central Dependency Track instance.
type DefaultPostProcess struct {
	ApiEndpoint string
	ApiKey      string
	Templates   Templates
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
