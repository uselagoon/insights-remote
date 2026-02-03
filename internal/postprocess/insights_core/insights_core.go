package insightcore

import (
	"encoding/json"
	"fmt"

	"lagoon.sh/insights-remote/internal"
	"lagoon.sh/insights-remote/internal/postprocess"
)

// PostProcess will send insights for all namespaces to Lagoon core insights-handler.
type PostProcess struct {
	MessageQWriter func(data []byte) error
}

func NewPostProcessor(
	mqEnable bool,
	mqWriteObject func(data []byte) error,
) postprocess.PostProcessor {
	// Check if RabbitMQ is disabled.
	if !mqEnable {
		return nil
	}

	return &PostProcess{
		MessageQWriter: mqWriteObject,
	}
}

func (p *PostProcess) PostProcess(message internal.LagoonInsightsMessage) error {
	if enableInsightsCoreIntegration, exists := message.Annotations["core.insights.lagoon.sh/enabled"]; exists {
		if enableInsightsCoreIntegration == "false" {
			return nil
		}
	}

	marshalledData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("unable to marshall config data: %w", err)
	}

	err = p.MessageQWriter(marshalledData)
	if err != nil {
		return fmt.Errorf("unable to write to message broker: %w", err)
	}

	return nil
}
