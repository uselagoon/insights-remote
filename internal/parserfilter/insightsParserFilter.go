package parserfilter

import (
	cdx "github.com/CycloneDX/cyclonedx-go"
	// "github.com/uselagoon/machinery/utils/namespace"
	"log/slog"
	LagoonFact "lagoon.sh/insights-remote/internal"
)

// func processSbomInsightsData(h *Messaging, insights InsightsData, v string, apiClient graphql.Client, resource ResourceDestination) ([]LagoonFact, string, error) {

// 	source := fmt.Sprintf("insights:sbom:%s", resource.Service)
// 	logger := slog.With("ProjectName", resource.Project, "EnvironmentName", resource.Environment, "Source", source)

// 	if insights.InsightsType != Sbom {
// 		return []LagoonFact{}, "", nil
// 	}

// 	bom, err := getBOMfromPayload(v)
// 	if err != nil {
// 		return []LagoonFact{}, "", err
// 	}

// 	kubernetesNamespaceName := namespace.GenerateNamespaceName("", resource.Environment, resource.Project, "", "", false)
// 	_, environment, apiErr := determineResourceFromLagoonAPIByKubernetesNamespace(apiClient, kubernetesNamespaceName)
// 	if apiErr != nil {
// 		return nil, "", apiErr
// 	}

// 	// we process the SBOM here
// 	// TODO: This should actually live in its own function somewhere else.
// 	if h.ProblemsFromSBOM == true {
// 		isAlive, err := IsTrivyServerIsAlive(h.TrivyServerEndpoint)
// 		if err != nil {
// 			return nil, "", fmt.Errorf("trivy server not alive: %v", err.Error())
// 		} else {
// 			logger.Debug("Trivy is reachable")
// 		}
// 		if isAlive {
// 			err = SbomToProblems(apiClient, h.TrivyServerEndpoint, "/tmp/", environment.Id, resource.Service, *bom)
// 		}
// 		if err != nil {
// 			return nil, "", err
// 		}
// 	}

// 	// Process SBOM into facts
// 	facts := processFactsFromSBOM(logger, bom.Components, environment.Id, source)

// 	facts, err = KeyFactsFilter(facts)
// 	if err != nil {
// 		return nil, "", err
// 	}

// 	if len(facts) == 0 {
// 		return nil, "", fmt.Errorf("no facts to process")
// 	}

// 	//log.Printf("Successfully decoded SBOM of image %s with %s, found %d for '%s:%s'", bom.Metadata.Component.Name, (*bom.Metadata.Tools)[0].Name, len(*bom.Components), resource.Project, resource.Environment)
// 	logger.Info("Successfully decoded SBOM",
// 		"image", bom.Metadata.Component.Name,
// 		"fieldName", (*bom.Metadata.Tools.Components)[0].Name,
// 		"Length", len(*bom.Components),
// 	)

// 	return facts, source, nil
// }

func processFactsFromSBOM(logger *slog.Logger, facts *[]cdx.Component, environmentId string, source string) []LagoonFact.Fact {
	var factsInput []LagoonFact.Fact
	if facts == nil || len(*facts) == 0 {
		return factsInput
	}

	var filteredFacts []cdx.Component
	keyFactsExistMap := make(map[string]bool)

	// Filter key facts
	for _, v := range *facts {
		if _, ok := keyFactsExistMap[v.Name]; !ok {
			keyFactsExistMap[v.Name] = true
			filteredFacts = append(filteredFacts, v)
		}
	}

	for _, f := range filteredFacts {
		fact := LagoonFact.Fact{
			EnvironmentId: environmentId,
			Name:        f.Name,
			Value:       f.Version,
			Source:      source,
			Description: f.PackageURL,
			KeyFact:     false,
			Type:        FactTypeText,
		}
		//if EnableDebug {
		//	log.Println("[DEBUG] processing fact name " + f.Name)
		//}
		logger.Debug("Processing fact",
			"Name", f.Name,
			"Value", f.Version,
		)
		fact, _ = ProcessLagoonFactAgainstRegisteredFilters(fact, f)
		factsInput = append(factsInput, fact)
	}
	return factsInput
}
