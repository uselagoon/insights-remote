package service

const deleteFactsType = "direct.delete.facts"
const deleteProblemsType = "direct.delete.problems"

type DirectDeleteMessage struct {
	Type          string `json:"type"`
	EnvironmentId int    `json:"environment"`
	Source        string `json:"source"`
	Service       string `json:"service"`
}
