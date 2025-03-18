package internal

type Fact struct {
	EnvironmentId   string `json:"environment"`
	ProjectName     string `json:"projectName"`
	EnvironmentName string `json:"environmentName"`
	Name            string `json:"name"`
	Value           string `json:"value"`
	Source          string `json:"source"`
	Description     string `json:"description"`
	Type            string `json:"type"`
	Category        string `json:"category"`
	Service         string `json:"service"`
}

type Facts struct {
	EnvironmentId   int    `json:"environment"`
	ProjectName     string `json:"projectName"`
	EnvironmentName string `json:"environmentName"`
	Facts           []Fact `json:"facts"`
	Type            string `json:"type"`
}

type ProblemSeverityRating string

type Problem struct {
	EnvironmentId     int                   `json:"environment"`
	Identifier        string                `json:"identifier"`
	Version           string                `json:"version,omitempty"`
	FixedVersion      string                `json:"fixedVersion,omitempty"`
	Source            string                `json:"source,omitempty"`
	Service           string                `json:"service,omitempty"`
	Data              string                `json:"data"`
	Severity          ProblemSeverityRating `json:"severity,omitempty"`
	SeverityScore     float64               `json:"severityScore,omitempty"`
	AssociatedPackage string                `json:"associatedPackage,omitempty"`
	Description       string                `json:"description,omitempty"`
	Links             string                `json:"links,omitempty"`
}

type Problems struct {
	EnvironmentId   int       `json:"environment"`
	ProjectName     string    `json:"projectName"`
	EnvironmentName string    `json:"environmentName"`
	Problems        []Problem `json:"problems"`
	Type            string    `json:"type"`
}

type LagoonInsightsMessage struct {
	Payload       map[string]string `json:"payload"`
	BinaryPayload map[string][]byte `json:"binaryPayload"`
	Annotations   map[string]string `json:"annotations"`
	Labels        map[string]string `json:"labels"`
	Environment   string            `json:"environment"`
	Project       string            `json:"project"`
	Type          string            `json:"type"`
}

// Insights Type consts
const InsightsTypeSBOM = "sbom"
const InsightsTypeInspect = "inspect"
const InsightsTypeUnclassified = "unclassified"

const InsightsLabel = "lagoon.sh/insightsType"
const InsightsUpdatedAnnotationLabel = "lagoon.sh/insightsProcessed"
const InsightsWriteDeferred = "lagoon.sh/insightsWriteDeferred"
