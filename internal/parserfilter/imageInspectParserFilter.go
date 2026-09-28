package parserfilter

import (
	"encoding/json"
	"log/slog"
	"strings"

	LagoonFact "lagoon.sh/insights-remote/internal"
)

// This becomes/implements the ParserFilter interface
type ImageData struct {
	Name          string            `json:"name"`
	Digest        string            `json:"digest"`
	RepoTags      []string          `json:"repoTags"`
	Created       string            `json:"created"`
	DockerVersion string            `json:"dockerVersion"`
	Labels        map[string]string `json:"labels"`
	Architecture  string            `json:"architecture"`
	OS            string            `json:"os"`
	Layers        []string          `json:"layers"`
	Env           []string          `json:"env"`
}

func ProcessImageInspectData(v string, logger *slog.Logger, environmentId string, source string) ([]LagoonFact.Fact, string, error) {
	decoded, err := decodeGzipString(v)
	if err != nil {
		return nil, "", err
	}

	marshallDecoded, err := json.Marshal(decoded)
	var imageInspect ImageData

	err = json.Unmarshal(marshallDecoded, &imageInspect)
	if err != nil {
		return nil, "", err
	}

	facts, err := processFactsFromImageInspect(logger, imageInspect, environmentId, source)
	if err != nil {
		return nil, "", err
	}
	logger.Info("Successfully decoded image-inspect")
	facts, err = KeyFactsFilter(facts)
	if err != nil {
		return nil, "", err
	}

	return facts, source, nil
}

func processFactsFromImageInspect(logger *slog.Logger, imageInspectData ImageData, environmentId string, source string) ([]LagoonFact.Fact, error) {

	var factsInput []LagoonFact.Fact

	var filteredFacts []EnvironmentVariable
	keyFactsExistMap := make(map[string]bool)

	// Check if image inspect contains useful environment variables
	if imageInspectData.Env != nil {
		for _, v := range imageInspectData.Env {
			logger.Debug("Processing env data", "data", v)
			var envSplitStr = strings.Split(v, "=")
			env := EnvironmentVariable{
				Key:   envSplitStr[0],
				Value: envSplitStr[1],
			}

			// Remove duplicate key facts
			if _, ok := keyFactsExistMap[env.Key]; !ok {
				keyFactsExistMap[env.Key] = true
				filteredFacts = append(filteredFacts, env)
			}
		}
	}

	for _, f := range filteredFacts {

		fact := LagoonFact.Fact{
			EnvironmentId: environmentId,
			Name:          f.Key,
			Value:         f.Value,
			Source:        source,
			Description:   "Environment Variable",
			KeyFact:       false,
			Type:          FactTypeText,
		}

		logger.Debug("Processing environment fact", "name", f.Key, "value", f.Value)

		fact, _ = ProcessLagoonFactAgainstRegisteredFilters(fact, f)
		factsInput = append(factsInput, fact)
	}
	return factsInput, nil
}
