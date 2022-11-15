package internal

type Fact struct {
	Environment string `json:"environment"`
	Name        string `json:"name"`
	Value       string `json:"value"`
	Source      string `json:"source"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Category    string `json:"category"`
	Service     string `json:"service"`
}

type Facts struct {
	Facts []Fact `json:"facts"`
}
