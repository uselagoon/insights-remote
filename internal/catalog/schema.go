package catalog

import (
	"context"
	"database/sql"
	"fmt"
)

// schemaArtifacts tracks each received SBOM (or, in future, image-scan) at a
// high level: the Lagoon project/environment/namespace it belongs to, and
// where its content lives on disk. The content itself is never stored here.
const schemaArtifacts = `
CREATE TABLE IF NOT EXISTS artifacts (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid                  TEXT NOT NULL UNIQUE,
    artifact_type         TEXT NOT NULL CHECK (artifact_type IN ('sbom', 'image-scan')),

    lagoon_project        TEXT NOT NULL,
    lagoon_environment    TEXT NOT NULL,
    lagoon_environment_id INTEGER,
    namespace             TEXT NOT NULL,
    service               TEXT,

    source                TEXT,
    storage_path          TEXT NOT NULL,
    content_sha256        TEXT,
    size_bytes            INTEGER,

    received_at           TEXT NOT NULL,
    deleted_at            TEXT
);
`

const schemaArtifactsIndexes = `
CREATE INDEX IF NOT EXISTS idx_artifacts_namespace   ON artifacts(namespace);
CREATE INDEX IF NOT EXISTS idx_artifacts_project_env ON artifacts(lagoon_project, lagoon_environment);
CREATE INDEX IF NOT EXISTS idx_artifacts_type        ON artifacts(artifact_type);
`

// schemaPostprocessRuns is the 1-to-many child of artifacts tracking every
// post-processing attempt (one row per processor per attempt), mirroring the
// existing success/failure/retry pattern used for ConfigMap-based insights.
const schemaPostprocessRuns = `
CREATE TABLE IF NOT EXISTS postprocess_runs (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    artifact_id     INTEGER NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    processor_label TEXT NOT NULL,
    attempt_number  INTEGER NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('pending', 'success', 'failure')),
    error_message   TEXT,
    started_at      TEXT NOT NULL,
    finished_at     TEXT,
    UNIQUE (artifact_id, processor_label, attempt_number)
);
`

const schemaPostprocessRunsIndexes = `
CREATE INDEX IF NOT EXISTS idx_postprocess_artifact     ON postprocess_runs(artifact_id);
CREATE INDEX IF NOT EXISTS idx_postprocess_label_status ON postprocess_runs(processor_label, status);
`

// schemaPostprocessStatusView is a convenience view returning only the
// latest attempt per (artifact, processor) pair, so callers don't need to
// know how to find "the current status" themselves.
const schemaPostprocessStatusView = `
CREATE VIEW IF NOT EXISTS artifact_postprocess_status AS
SELECT pr.*
FROM postprocess_runs pr
JOIN (
    SELECT artifact_id, processor_label, MAX(attempt_number) AS max_attempt
    FROM postprocess_runs
    GROUP BY artifact_id, processor_label
) latest
  ON pr.artifact_id     = latest.artifact_id
 AND pr.processor_label = latest.processor_label
 AND pr.attempt_number  = latest.max_attempt;
`

// migrate creates the catalog schema if it doesn't already exist. There is
// no migration framework at this stage - every statement here must be safe
// to run repeatedly (CREATE ... IF NOT EXISTS) against an existing database,
// so this can simply be called every time the service starts.
func migrate(ctx context.Context, db *sql.DB) error {
	statements := []string{
		schemaArtifacts,
		schemaArtifactsIndexes,
		schemaPostprocessRuns,
		schemaPostprocessRunsIndexes,
		schemaPostprocessStatusView,
	}

	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("catalog: migration failed: %w", err)
		}
	}
	return nil
}
