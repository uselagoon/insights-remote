package service

type DirectDeleteMessage struct {
	Type          string `json:"type"`
	EnvironmentId int    `json:"environmentId"`
	Source        string `json:"source"`
	Service       string `json:"service"`
}
