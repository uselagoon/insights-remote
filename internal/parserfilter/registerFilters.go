package parserfilter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	LagoonFact "lagoon.sh/insights-remote/internal"
)

var KeyFactFilters []func(filter parserFilter) parserFilter

type FactTransformNameValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type FactLookupNameValue struct {
	Name       string `json:"name"`
	Value      string `json:"value"`
	ExactMatch bool   `json:"exactMatch" yaml:"exactMatch"`
}

type FactTransform struct {
	Type            string                   `json:"type"`
	Lookupvalue     []FactLookupNameValue    `json:"lookupvalue"`
	Transformations []FactTransformNameValue `json:"transformations"`
	Keyfact         bool                     `json:"keyfact"`
}

type FactTransforms struct {
	Transforms []FactTransform `json:"transforms"`
}

// registerFilter takes in a parserFilter defining function (capturing a transform programmatically) and adds it to
// the global list of parserFilters
func registerFilter(filter func(filter parserFilter) parserFilter) {
	KeyFactFilters = append(KeyFactFilters, filter)
}

// ProcessLagoonFactAgainstRegisteredFilters will take in a single fact and run it against all KeyFactFilters
func ProcessLagoonFactAgainstRegisteredFilters(fact LagoonFact.Fact, insightsRawData interface{}) (LagoonFact.Fact, error) {
	for _, filter := range KeyFactFilters {
		pf := FactProcessor{
			Fact:          fact,
			InsightsData:  insightsRawData,
			hasErrorState: false,
			theError:      nil,
			filteredOut:   false,
		}
		pfout := filter(&pf)
		if !pfout.isFilteredOut() {
			fact = pfout.getFact()
		}

	}
	return fact, nil
}

func LoadTransformsFromDisk(filename string) ([]FactTransform, error) {
	ret := FactTransforms{}

	file, err := os.ReadFile(filename)

	if err != nil {
		return ret.Transforms, err
	}

	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".json":
		err = json.Unmarshal(file, &ret)
	case ".yaml", ".yml":
		err = yaml.Unmarshal(file, &ret)
	default:
		err = fmt.Errorf("Unsupported file type for default transforms: %v", ext)
	}

	if err != nil {
		return ret.Transforms, err
	}

	return ret.Transforms, nil
}

// GenerateFilterFromTransform will take a transform description and generate a function that will check a fact
// against a transform - that is, see if we need to change its name/value.
// Perhaps the only tricky thing here is that
func GenerateFilterFromTransform(transform FactTransform) (func(filter parserFilter) parserFilter, error) {

	// we build and return a function that captures the given transform (description of changes) in a closure
	return func(filter parserFilter) parserFilter {

		if transform.Type != "" {
			filter = filter.isOfType(transform.Type)
		}

		for i := range transform.Lookupvalue {

			n := transform.Lookupvalue[i].Name
			v := transform.Lookupvalue[i].Value
			em := transform.Lookupvalue[i].ExactMatch
			if em {
				filter = filter.fieldContainsExactMatch(n, v)
			} else {
				filter = filter.fieldContains(n, v)
			}

		}

		for i := range transform.Transformations {
			n := transform.Transformations[i].Name
			v := transform.Transformations[i].Value
			filter = filter.setFactField(n, v)
		}

		if transform.Keyfact {
			filter = filter.setKeyFact(true)
		}

		return filter
	}, nil

}

func RegisterFiltersFromDisk(filename string) error {

	transforms, err := LoadTransformsFromDisk(filename)
	if err != nil {
		return err
	}
	for _, transform := range transforms {
		filter, _ := GenerateFilterFromTransform(transform)
		registerFilter(filter)
	}

	return nil
}
