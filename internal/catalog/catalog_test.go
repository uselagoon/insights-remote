package catalog

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nested", "catalog.sqlite")
	db, err := Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestOpenCreatesSchemaAndParentDir(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, "SELECT 1 FROM artifacts LIMIT 0")
	assert.NoError(t, err)

	_, err = db.ExecContext(ctx, "SELECT 1 FROM postprocess_runs LIMIT 0")
	assert.NoError(t, err)

	_, err = db.ExecContext(ctx, "SELECT 1 FROM artifact_postprocess_status LIMIT 0")
	assert.NoError(t, err)
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.sqlite")

	db, err := Open(context.Background(), path)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// Re-opening (and therefore re-running migrate) against an existing
	// database file must not error.
	db2, err := Open(context.Background(), path)
	require.NoError(t, err)
	defer func() { _ = db2.Close() }()
}

func TestCreateAndGetArtifact(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	id, err := db.CreateArtifact(ctx, Artifact{
		ArtifactType:        ArtifactTypeSBOM,
		LagoonProject:       "test-project",
		LagoonEnvironment:   "test-environment",
		LagoonEnvironmentID: 777,
		Namespace:           "testns",
		Service:             "test-service",
		Source:              "InsightsRemoteWebService",
		StoragePath:         "/data/sboms/testns/test-test-sbom-json",
		ContentSHA256:       "deadbeef",
		SizeBytes:           42,
	})
	require.NoError(t, err)
	assert.NotZero(t, id)

	got, err := db.GetArtifact(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, ArtifactTypeSBOM, got.ArtifactType)
	assert.Equal(t, "test-project", got.LagoonProject)
	assert.Equal(t, "test-environment", got.LagoonEnvironment)
	assert.Equal(t, 777, got.LagoonEnvironmentID)
	assert.Equal(t, "testns", got.Namespace)
	assert.Equal(t, "test-service", got.Service)
	assert.Equal(t, int64(42), got.SizeBytes)
	assert.NotEmpty(t, got.UUID)
	assert.False(t, got.ReceivedAt.IsZero())
	assert.Nil(t, got.DeletedAt)

	byUUID, err := db.GetArtifactByUUID(ctx, got.UUID)
	require.NoError(t, err)
	assert.Equal(t, got.ID, byUUID.ID)
}

func TestGetArtifactNotFound(t *testing.T) {
	db := openTestDB(t)

	_, err := db.GetArtifact(context.Background(), 99999)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestPostProcessRunLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	artifactID, err := db.CreateArtifact(ctx, Artifact{
		ArtifactType:      ArtifactTypeSBOM,
		LagoonProject:     "test-project",
		LagoonEnvironment: "test-environment",
		Namespace:         "testns",
		StoragePath:       "/data/sboms/testns/whatever",
	})
	require.NoError(t, err)

	run1, err := db.StartPostProcessRun(ctx, artifactID, "dependency-track")
	require.NoError(t, err)
	assert.Equal(t, 1, run1.AttemptNumber)
	assert.Equal(t, PostProcessStatusPending, run1.Status)

	require.NoError(t, db.CompletePostProcessRun(ctx, run1.ID, PostProcessStatusFailure, "boom"))

	run2, err := db.StartPostProcessRun(ctx, artifactID, "dependency-track")
	require.NoError(t, err)
	assert.Equal(t, 2, run2.AttemptNumber, "attempt numbers should increment per (artifact, processor) pair")

	require.NoError(t, db.CompletePostProcessRun(ctx, run2.ID, PostProcessStatusSuccess, ""))

	latest, err := db.LatestPostProcessRun(ctx, artifactID, "dependency-track")
	require.NoError(t, err)
	assert.Equal(t, 2, latest.AttemptNumber)
	assert.Equal(t, PostProcessStatusSuccess, latest.Status)
	require.NotNil(t, latest.FinishedAt)

	// A second, independent processor against the same artifact starts its
	// own attempt sequence from 1.
	otherRun, err := db.StartPostProcessRun(ctx, artifactID, "core.insights.lagoon.sh/status")
	require.NoError(t, err)
	assert.Equal(t, 1, otherRun.AttemptNumber)
}

func TestLatestPostProcessRunNotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	artifactID, err := db.CreateArtifact(ctx, Artifact{
		ArtifactType:      ArtifactTypeSBOM,
		LagoonProject:     "test-project",
		LagoonEnvironment: "test-environment",
		Namespace:         "testns",
		StoragePath:       "/data/sboms/testns/whatever",
	})
	require.NoError(t, err)

	_, err = db.LatestPostProcessRun(ctx, artifactID, "never-ran")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}
