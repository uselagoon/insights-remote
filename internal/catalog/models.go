package catalog

import "time"

// ArtifactType identifies what kind of insight artifact a row represents.
// Only ArtifactTypeSBOM is written today (via POST /sboms); ArtifactTypeImageScan
// is reserved for the skopeo image-scan ingestion path that will follow the
// same pattern later.
type ArtifactType string

const (
	ArtifactTypeSBOM      ArtifactType = "sbom"
	ArtifactTypeImageScan ArtifactType = "image-scan"
)

// PostProcessStatus mirrors the success/failure/retry states already used by
// the ConfigMap-based postprocess pipeline (see
// internal/controller/configmap_controller.go)
type PostProcessStatus string

const (
	PostProcessStatusPending PostProcessStatus = "pending"
	PostProcessStatusSuccess PostProcessStatus = "success"
	PostProcessStatusFailure PostProcessStatus = "failure"
)

// Artifact is the high-level record of a single received SBOM (or, in
// future, skopeo image scan) submission. It tracks where the artifact lives
// on disk and the Lagoon/k8s context it was received under - it does not
// store the artifact content itself.
type Artifact struct {
	ID                  int64
	UUID                string // stable external identifier, generated if not supplied (not implemented)
	ArtifactType        ArtifactType
	LagoonProject       string
	LagoonEnvironment   string
	LagoonEnvironmentID int
	Namespace           string // JWT-derived, trusted
	Service             string // lagoon.sh/service, may be empty

	Source        string // e.g. "InsightsRemoteWebService"
	StoragePath   string // on-disk location written by the /sboms handler
	ContentSHA256 string
	SizeBytes     int64

	ReceivedAt time.Time
	DeletedAt  *time.Time // soft-delete, mirrors the burn-after-reading pattern
}

// PostProcessRun is a single post-processing attempt against an Artifact.
// There can be many of these per Artifact: one per processor
// (postprocess.PostProcessor.Label()), and more than one per processor if
// retries occurred.
type PostProcessRun struct {
	ID             int64
	ArtifactID     int64
	ProcessorLabel string // matches postprocess.PostProcessor.Label()
	AttemptNumber  int    // 1, 2, 3... per (ArtifactID, ProcessorLabel)
	Status         PostProcessStatus
	ErrorMessage   string
	StartedAt      time.Time
	FinishedAt     *time.Time
}
