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
	Source          string `json:"source"`
	Facts           []Fact `json:"facts"`
	Type            string `json:"type"`
	InsightsType    string `json:"insightsType"`
}
