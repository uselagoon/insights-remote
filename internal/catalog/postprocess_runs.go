package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// StartPostProcessRun records a new post-processing attempt for an artifact
// against a given processor (processorLabel should match
// postprocess.PostProcessor.Label()). The attempt number is automatically
// set to one more than the highest existing attempt for this
// (artifact, processor) pair, so retries accumulate rather than overwrite.
func (db *DB) StartPostProcessRun(ctx context.Context, artifactID int64, processorLabel string) (*PostProcessRun, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("catalog: unable to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var maxAttempt sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		SELECT MAX(attempt_number) FROM postprocess_runs
		WHERE artifact_id = ? AND processor_label = ?`,
		artifactID, processorLabel,
	).Scan(&maxAttempt)
	if err != nil {
		return nil, fmt.Errorf("catalog: unable to determine next attempt number: %w", err)
	}

	run := PostProcessRun{
		ArtifactID:     artifactID,
		ProcessorLabel: processorLabel,
		AttemptNumber:  int(maxAttempt.Int64) + 1,
		Status:         PostProcessStatusPending,
		StartedAt:      time.Now().UTC(),
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO postprocess_runs (artifact_id, processor_label, attempt_number, status, started_at)
		VALUES (?, ?, ?, ?, ?)`,
		run.ArtifactID, run.ProcessorLabel, run.AttemptNumber, string(run.Status),
		run.StartedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, fmt.Errorf("catalog: unable to insert postprocess run: %w", err)
	}

	run.ID, err = res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("catalog: unable to read inserted postprocess run id: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("catalog: unable to commit postprocess run: %w", err)
	}

	return &run, nil
}

// CompletePostProcessRun marks an in-flight post-processing attempt as
// finished, with either success or failure and an optional error message -
// mirroring the successfulPostProcessRun/failedPostProcessRun outcomes
// already used by the ConfigMap-based pipeline.
func (db *DB) CompletePostProcessRun(ctx context.Context, runID int64, status PostProcessStatus, errMsg string) error {
	if status != PostProcessStatusSuccess && status != PostProcessStatusFailure {
		return fmt.Errorf("catalog: invalid terminal status %q", status)
	}

	_, err := db.ExecContext(ctx, `
		UPDATE postprocess_runs
		SET status = ?, error_message = ?, finished_at = ?
		WHERE id = ?`,
		string(status), errMsg, time.Now().UTC().Format(time.RFC3339Nano), runID,
	)
	if err != nil {
		return fmt.Errorf("catalog: unable to complete postprocess run %d: %w", runID, err)
	}
	return nil
}

// LatestPostProcessRun returns the most recent attempt for a given
// (artifact, processor) pair, or sql.ErrNoRows if there have been none.
func (db *DB) LatestPostProcessRun(ctx context.Context, artifactID int64, processorLabel string) (*PostProcessRun, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, artifact_id, processor_label, attempt_number, status, error_message, started_at, finished_at
		FROM postprocess_runs
		WHERE artifact_id = ? AND processor_label = ?
		ORDER BY attempt_number DESC
		LIMIT 1`,
		artifactID, processorLabel,
	)

	var (
		run          PostProcessRun
		status       string
		errorMessage sql.NullString
		startedAt    string
		finishedAt   sql.NullString
	)

	err := row.Scan(&run.ID, &run.ArtifactID, &run.ProcessorLabel, &run.AttemptNumber,
		&status, &errorMessage, &startedAt, &finishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("catalog: unable to scan postprocess run: %w", err)
	}

	run.Status = PostProcessStatus(status)
	run.ErrorMessage = errorMessage.String

	run.StartedAt, err = time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return nil, fmt.Errorf("catalog: unable to parse started_at: %w", err)
	}
	if finishedAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, finishedAt.String)
		if err != nil {
			return nil, fmt.Errorf("catalog: unable to parse finished_at: %w", err)
		}
		run.FinishedAt = &t
	}

	return &run, nil
}
