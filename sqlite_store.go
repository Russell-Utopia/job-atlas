package jobatlas

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const discoverySchema = `
-- A discovery run is the stable object returned to the caller.
CREATE TABLE IF NOT EXISTS discovery_runs (
    run_id     TEXT PRIMARY KEY,
    status     TEXT NOT NULL CHECK (status IN ('running', 'completed', 'blocked')),
    error      TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

-- One work item records coverage for one enabled source in one requested city.
CREATE TABLE IF NOT EXISTS discovery_work_items (
    run_id          TEXT NOT NULL REFERENCES discovery_runs(run_id) ON DELETE CASCADE,
    source_id       TEXT NOT NULL,
    city            TEXT NOT NULL,
    city_position   INTEGER NOT NULL,
    source_position INTEGER NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed')),
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    PRIMARY KEY (run_id, source_id, city)
);

CREATE INDEX IF NOT EXISTS discovery_work_items_next
    ON discovery_work_items(status, created_at, run_id, city_position, source_position);
`

type discoveryStore struct {
	db *sql.DB
}

type workItem struct {
	runID RunID
	task  companyDiscoveryTask
}

func openDiscoveryStore(path string) (*discoveryStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database path must not be empty")
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &discoveryStore{db: db}
	if err := store.initialize(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *discoveryStore) initialize(ctx context.Context) error {
	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		discoverySchema,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize sqlite: %w", err)
		}
	}
	return nil
}

func (s *discoveryStore) close() error {
	return s.db.Close()
}

func (s *discoveryStore) unfinishedSourceIDs(ctx context.Context) ([]SourceID, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT source_id
		FROM discovery_work_items
		WHERE status <> 'completed'
		ORDER BY source_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sourceIDs []SourceID
	for rows.Next() {
		var id SourceID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		sourceIDs = append(sourceIDs, id)
	}
	return sourceIDs, rows.Err()
}

func (s *discoveryStore) restoreInterruptedWork(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE discovery_work_items
		SET status = 'pending', updated_at = ?
		WHERE status = 'running'
	`, unixMillis())
	return err
}

func (s *discoveryStore) createRun(
	ctx context.Context,
	runID RunID,
	scopes []CityScope,
	sourceIDs []SourceID,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := unixMillis()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO discovery_runs(run_id, status, error, created_at, updated_at)
		VALUES (?, 'running', NULL, ?, ?)
	`, runID, now, now); err != nil {
		return err
	}

	for cityPosition, scope := range scopes {
		for sourcePosition, sourceID := range sourceIDs {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO discovery_work_items(
					run_id, source_id, city, city_position, source_position,
					status, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, 'pending', ?, ?)
			`, runID, sourceID, scope.City, cityPosition, sourcePosition, now, now); err != nil {
				return err
			}
		}
	}

	if len(sourceIDs) == 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE discovery_runs
			SET status = 'completed', updated_at = ?
			WHERE run_id = ?
		`, now, runID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *discoveryStore) getDiscovery(ctx context.Context, runID RunID) (Discovery, error) {
	var status Status
	var taskError sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT status, error
		FROM discovery_runs
		WHERE run_id = ?
	`, runID).Scan(&status, &taskError)
	if errors.Is(err, sql.ErrNoRows) {
		return Discovery{}, ErrRunNotFound
	}
	if err != nil {
		return Discovery{}, err
	}

	discovery := Discovery{Status: status, Jobs: []Job{}}
	if taskError.Valid {
		discovery.Error = &taskError.String
	}
	return discovery, nil
}

func (s *discoveryStore) claimWork(ctx context.Context) (workItem, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workItem{}, false, err
	}
	defer tx.Rollback()

	var work workItem
	err = tx.QueryRowContext(ctx, `
		SELECT work.run_id, work.source_id, work.city
		FROM discovery_work_items AS work
		JOIN discovery_runs AS run ON run.run_id = work.run_id
		WHERE work.status = 'pending' AND run.status = 'running'
		ORDER BY work.created_at, work.run_id, work.city_position, work.source_position
		LIMIT 1
	`).Scan(&work.runID, &work.task.sourceID, &work.task.scope.City)
	if errors.Is(err, sql.ErrNoRows) {
		return workItem{}, false, nil
	}
	if err != nil {
		return workItem{}, false, err
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE discovery_work_items
		SET status = 'running', updated_at = ?
		WHERE run_id = ? AND source_id = ? AND city = ? AND status = 'pending'
	`, unixMillis(), work.runID, work.task.sourceID, work.task.scope.City)
	if err != nil {
		return workItem{}, false, err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return workItem{}, false, err
	}
	if updated != 1 {
		return workItem{}, false, errors.New("pending discovery work was not claimed")
	}

	if err := tx.Commit(); err != nil {
		return workItem{}, false, err
	}
	return work, true, nil
}

func (s *discoveryStore) completeWork(ctx context.Context, work workItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := unixMillis()
	result, err := tx.ExecContext(ctx, `
		UPDATE discovery_work_items
		SET status = 'completed', updated_at = ?
		WHERE run_id = ? AND source_id = ? AND city = ? AND status = 'running'
	`, now, work.runID, work.task.sourceID, work.task.scope.City)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return errors.New("running discovery work was not completed")
	}

	var unfinished bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM discovery_work_items
			WHERE run_id = ? AND status <> 'completed'
		)
	`, work.runID).Scan(&unfinished); err != nil {
		return err
	}
	if !unfinished {
		if _, err := tx.ExecContext(ctx, `
			UPDATE discovery_runs
			SET status = 'completed', error = NULL, updated_at = ?
			WHERE run_id = ?
		`, now, work.runID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func unixMillis() int64 {
	return time.Now().UTC().UnixMilli()
}
