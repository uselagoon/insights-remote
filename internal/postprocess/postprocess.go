package postprocess

import (
	"lagoon.sh/insights-remote/internal"
)

type PostProcessor interface {
	// PostProcess processes insights. Must be idempotent.
	PostProcess(message internal.LagoonInsightsMessage) error
}
