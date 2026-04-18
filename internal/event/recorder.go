package event

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/phuc-nt/dandori-cli/internal/db"
	"github.com/phuc-nt/dandori-cli/internal/model"
	"github.com/phuc-nt/dandori-cli/internal/util"
)

type Recorder struct {
	db *db.LocalDB
}

func NewRecorder(localDB *db.LocalDB) *Recorder {
	return &Recorder{db: localDB}
}

func (r *Recorder) RecordEvent(runID string, layer model.EventLayer, eventType string, data any) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal event data: %w", err)
	}

	_, err = r.db.Exec(`
		INSERT INTO events (run_id, layer, event_type, data, ts)
		VALUES (?, ?, ?, ?, ?)
	`, runID, layer, eventType, string(dataJSON), time.Now().Format(time.RFC3339))

	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	return nil
}

func (r *Recorder) RecordAuditEvent(actor string, action model.AuditAction, entityType, entityID string, details any) error {
	var lastHash string
	r.db.QueryRow(`SELECT curr_hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&lastHash)

	detailsJSON, _ := json.Marshal(details)
	ts := time.Now()

	currHash := util.ComputeAuditHash(
		lastHash,
		actor,
		string(action),
		entityType,
		entityID,
		string(detailsJSON),
		ts,
	)

	_, err := r.db.Exec(`
		INSERT INTO audit_log (prev_hash, curr_hash, actor, action, entity_type, entity_id, details, ts)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, lastHash, currHash, actor, action, entityType, entityID, string(detailsJSON), ts.Format(time.RFC3339))

	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}

	return nil
}

func (r *Recorder) GetUnsyncedEvents(limit int) ([]model.Event, error) {
	rows, err := r.db.Query(`
		SELECT id, run_id, layer, event_type, data, ts
		FROM events
		WHERE synced = 0
		ORDER BY id
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []model.Event
	for rows.Next() {
		var e model.Event
		var ts string
		if err := rows.Scan(&e.ID, &e.RunID, &e.Layer, &e.EventType, &e.Data, &ts); err != nil {
			continue
		}
		e.Timestamp, _ = time.Parse(time.RFC3339, ts)
		events = append(events, e)
	}

	return events, nil
}

func (r *Recorder) MarkEventsSynced(ids []int64) error {
	for _, id := range ids {
		if _, err := r.db.Exec(`UPDATE events SET synced = 1 WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}
