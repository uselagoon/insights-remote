package parserfilter

import LagoonFact "lagoon.sh/insights-remote/internal"

// KeyFactsFilter simply takes a slice of LagoonFacts and filters out anything not marked with KeyFact = true
// This is used downstream to ensure only key facts are written to the DB
func KeyFactsFilter(factsInput []LagoonFact.Fact) ([]LagoonFact.Fact, error) {
	var filteredFacts []LagoonFact.Fact
	for _, v := range factsInput {
		if v.KeyFact {
			filteredFacts = append(filteredFacts, v)
		}
	}
	return filteredFacts, nil
}
