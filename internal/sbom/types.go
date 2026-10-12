package sbom

type BOM struct {
	Schema         string      `json:"schema,omitempty"`
	BOMFormat      string      `json:"bomFormat,omitempty"`
	SpecVersion    string      `json:"specVersion,omitempty"`
	SerialNumber   string      `json:"serialNumber,omitempty"`
	Version        int         `json:"version,omitempty"`
	Metadata       *Metadata   `json:"metadata,omitempty"`
	Components     []Component `json:"components,omitempty"`
	Dependencies   []any       `json:"dependencies,omitempty"`
	Compositions   []any       `json:"compositions,omitempty"`
	Vulnerabilites []any       `json:"vulnerabilities,omitempty"`
}

type Metadata struct {
	Timestamp string     `json:"timestamp,omitempty"`
	Tools     any        `json:"tools,omitempty"`
	Component *Component `json:"component,omitempty"`
}

type Component struct {
	BOMRef             string     `json:"bom-ref,omitempty"`
	Type               string     `json:"type,omitempty"`
	Name               string     `json:"name,omitempty"`
	Version            string     `json:"version,omitempty"`
	CPE                string     `json:"cpe,omitempty"`
	PURL               string     `json:"purl,omitempty"`
	Properties         []Property `json:"properties,omitempty"`
	Author             string     `json:"author,omitempty"`
	Publisher          string     `json:"publisher,omitempty"`
	Description        string     `json:"description,omitempty"`
	Licenses           []any      `json:"licenses,omitempty"`
	Hashes             []any      `json:"hashes,omitempty"`
	ExternalReferences []any      `json:"externalReferences,omitempty"`
}

type Property struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

const CPE23PropertyName = "syft:cpe23"
