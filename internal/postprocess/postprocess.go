package postprocess

import (
	"lagoon.sh/insights-remote/internal"
)

type PostProcessor interface {
	// PostProcess processes insights for 3rd party tools. Must be idempotent.
	PostProcess(message internal.LagoonInsightsMessage) error
}

type PostProcessors struct {
	PostProcessors []PostProcessor
}
