package sbom

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

type ExpandResult struct {
	Data            []byte
	ComponentsAdded int
	Changes         []string
}

const ExpandedFromPropertyName = "insights-remote:expanded-from"

func Expand(data []byte) (*ExpandResult, error) {
	var bom BOM
	if err := json.Unmarshal(data, &bom); err != nil {
		log.Printf("failed to parse json SBOM expand skipped: %v", err)
		return &ExpandResult{Data: data, ComponentsAdded: 0}, nil
	}

	result := &ExpandResult{}
	var newComponents []Component

	for i := range bom.Components {
		comp := &bom.Components[i]
		expanded := expandComponent(comp)
		for _, exp := range expanded {
			newComponents = append(newComponents, exp)
			result.Changes = append(result.Changes,
				fmt.Sprintf("%s: added %s", comp.Name, exp.CPE))
		}
	}

	if len(newComponents) == 0 {
		result.Data = data
		return result, nil
	}

	var original map[string]any
	if err := json.Unmarshal(data, &original); err != nil {
		return nil, fmt.Errorf("failed to parse data: %w", err)
	}
	comps, _ := original["components"].([]any)
	for _, c := range newComponents {
		comps = append(comps, c)
	}
	original["components"] = comps
	result.ComponentsAdded = len(newComponents)

	expanded, err := json.Marshal(original)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	result.Data = expanded
	return result, nil
}

func expandComponent(comp *Component) []Component {
	if comp.CPE == "" {
		return nil
	}

	if v, p := parseCPE(comp.CPE); v != "" && normalize(v) == normalize(p) {
		return nil
	}

	var syftProperties []string
	for _, prop := range comp.Properties {
		if prop.Name == CPE23PropertyName {
			syftProperties = append(syftProperties, prop.Value)
		}
	}

	if len(syftProperties) == 0 {
		return nil
	}

	var expanded []Component
	seen := make(map[string]bool)
	seen[comp.CPE] = true

	for _, cpe := range syftProperties {
		if seen[cpe] {
			continue
		}
		seen[cpe] = true

		vendor, product := parseCPE(cpe)
		if vendor == "" || product == "" {
			continue
		}

		if normalize(vendor) != normalize(product) || normalize(product) != normalize(comp.Name) {
			continue
		}

		expanded = append(expanded, Component{
			BOMRef:  fmt.Sprintf("%s-expanded-%d", comp.BOMRef, len(expanded)),
			Type:    comp.Type,
			Name:    fmt.Sprintf("%s%s", comp.Name, "(Child)"),
			Version: comp.Version,
			CPE:     cpe,
			Properties: []Property{{
				Name:  ExpandedFromPropertyName,
				Value: comp.CPE,
			}},
		})
	}

	return expanded
}

func parseCPE(cpe string) (vendor, product string) {
	parts := strings.Split(cpe, ":")
	if len(parts) < 5 {
		return "", ""
	}
	return parts[3], parts[4]
}

func normalize(field string) string {
	return strings.ToLower(strings.TrimSpace(field))
}
