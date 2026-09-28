package parserfilter

type EnvironmentVariable struct {
	Key   string
	Value string
}

const (
	FactTypeText   string = "TEXT"
	FactTypeUrl    string = "URL"
	FactTypeSemver string = "SEMVER"
)
