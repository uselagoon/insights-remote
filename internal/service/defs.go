package service

type DeleteFactsMessage struct {
	Type          string `json:"type"`
	EnvironmentId int    `json:"environmentId"`
	Source        string `json:"source"`
	Service       string `json:"service"`
}

type DeleteProblemsMessage struct {
	Type          string `json:"type"`
	EnvironmentId int    `json:"environmentId"`
	Source        string `json:"source"`
	Service       string `json:"service"`
}
