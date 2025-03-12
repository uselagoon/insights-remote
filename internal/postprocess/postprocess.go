package postprocess

import (
	"lagoon.sh/insights-remote/internal"
)

type PostProcessor interface {
	PostProcess(message internal.LagoonInsightsMessage) error
}

type PostProcessors struct {
	PostProcessors []PostProcessor
}
