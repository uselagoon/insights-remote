package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreateArtifact inserts a new artifact row. If a.UUID is empty, one is
// generated. If a.ReceivedAt is zero, the current time is used. Returns the
// generated row ID.
func (db *DB) CreateArtifact(ctx context.Context, a Artifact) (int64, error) {
	if a.UUID == "" {
		a.UUID = uuid.NewString()
	}
	if a.ReceivedAt.IsZero() {
		a.ReceivedAt = time.Now().UTC()
	}

	var deletedAt any
	if a.DeletedAt != nil {
		deletedAt = a.DeletedAt.UTC().Format(time.RFC3339Nano)
	}

	res, err := db.ExecContext(ctx, `
		INSERT INTO artifacts (
			uuid, artifact_type, lagoon_project, lagoon_environment, lagoon_environment_id,
			namespace, service, source, storage_path, content_sha256, size_bytes,
			received_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.UUID, string(a.ArtifactType), a.LagoonProject, a.LagoonEnvironment, a.LagoonEnvironmentID,
		a.Namespace, a.Service, a.Source, a.StoragePath, a.ContentSHA256, a.SizeBytes,
		a.ReceivedAt.UTC().Format(time.RFC3339Nano), deletedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("catalog: unable to insert artifact: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("catalog: unable to read inserted artifact id: %w", err)
	}
	return id, nil
}

const artifactColumns = `
	id, uuid, artifact_type, lagoon_project, lagoon_environment, lagoon_environment_id,
	namespace, service, source, storage_path, content_sha256, size_bytes,
	received_at, deleted_at`

// GetArtifact fetches a single artifact by its row ID.
func (db *DB) GetArtifact(ctx context.Context, id int64) (*Artifact, error) {
	row := db.QueryRowContext(ctx, `SELECT `+artifactColumns+` FROM artifacts WHERE id = ?`, id)
	return scanArtifact(row)
}

// GetArtifactByUUID fetches a single artifact by its external UUID.
func (db *DB) GetArtifactByUUID(ctx context.Context, id string) (*Artifact, error) {
	row := db.QueryRowContext(ctx, `SELECT `+artifactColumns+` FROM artifacts WHERE uuid = ?`, id)
	return scanArtifact(row)
}

func scanArtifact(row *sql.Row) (*Artifact, error) {
	var (
		a            Artifact
		artifactType string
		receivedAt   string
		deletedAt    sql.NullString
		lagoonEnvID  sql.NullInt64
		service      sql.NullString
		source       sql.NullString
		contentSHA   sql.NullString
		sizeBytes    sql.NullInt64
	)

	err := row.Scan(
		&a.ID, &a.UUID, &artifactType, &a.LagoonProject, &a.LagoonEnvironment, &lagoonEnvID,
		&a.Namespace, &service, &source, &a.StoragePath, &contentSHA, &sizeBytes,
		&receivedAt, &deletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("catalog: unable to scan artifact: %w", err)
	}

	a.ArtifactType = ArtifactType(artifactType)
	a.LagoonEnvironmentID = int(lagoonEnvID.Int64)
	a.Service = service.String
	a.Source = source.String
	a.ContentSHA256 = contentSHA.String
	a.SizeBytes = sizeBytes.Int64

	a.ReceivedAt, err = time.Parse(time.RFC3339Nano, receivedAt)
	if err != nil {
		return nil, fmt.Errorf("catalog: unable to parse received_at: %w", err)
	}
	if deletedAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, deletedAt.String)
		if err != nil {
			return nil, fmt.Errorf("catalog: unable to parse deleted_at: %w", err)
		}
		a.DeletedAt = &t
	}

	return &a, nil
}
