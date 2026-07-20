package storage

import (
	"database/sql"
	"fmt"
	"time"
)

type OutputRecord struct {
	ID         string    `json:"id"`
	JobID      string    `json:"job_id"`
	Type       string    `json:"type"`
	Path       string    `json:"path"`
	MimeType   string    `json:"mime_type"`
	SizeBytes  int64     `json:"size_bytes"`
	DurationMs *float64  `json:"duration_ms,omitempty"`
	SampleRate *int      `json:"sample_rate,omitempty"`
	Channels   *int      `json:"channels,omitempty"`
	Metadata   string    `json:"metadata,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type OutputsRepository struct {
	db *DB
}

func NewOutputsRepository(db *DB) *OutputsRepository {
	return &OutputsRepository{db: db}
}

func (r *OutputsRepository) Create(out *OutputRecord) error {
	_, err := r.db.db.Exec(
		`INSERT INTO outputs (id, job_id, type, path, mime_type, size_bytes,
		                      duration_ms, sample_rate, channels, metadata, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		out.ID, out.JobID, out.Type, out.Path, out.MimeType, out.SizeBytes,
		out.DurationMs, out.SampleRate, out.Channels, out.Metadata, out.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	return nil
}

func (r *OutputsRepository) Get(id string) (*OutputRecord, error) {
	row := r.db.db.QueryRow(
		`SELECT id, job_id, type, path, mime_type, size_bytes,
		        duration_ms, sample_rate, channels, metadata, created_at
		 FROM outputs WHERE id = ?`, id,
	)

	out := &OutputRecord{}
	err := row.Scan(
		&out.ID, &out.JobID, &out.Type, &out.Path, &out.MimeType, &out.SizeBytes,
		&out.DurationMs, &out.SampleRate, &out.Channels, &out.Metadata, &out.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("output not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get output %s: %w", id, err)
	}
	return out, nil
}

func (r *OutputsRepository) ListByJob(jobID string) ([]*OutputRecord, error) {
	rows, err := r.db.db.Query(
		`SELECT id, job_id, type, path, mime_type, size_bytes,
		        duration_ms, sample_rate, channels, metadata, created_at
		 FROM outputs WHERE job_id = ? ORDER BY created_at ASC`, jobID,
	)
	if err != nil {
		return nil, fmt.Errorf("list outputs by job %s: %w", jobID, err)
	}
	defer rows.Close()

	var outputs []*OutputRecord
	for rows.Next() {
		out := &OutputRecord{}
		if err := rows.Scan(
			&out.ID, &out.JobID, &out.Type, &out.Path, &out.MimeType, &out.SizeBytes,
			&out.DurationMs, &out.SampleRate, &out.Channels, &out.Metadata, &out.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan output: %w", err)
		}
		outputs = append(outputs, out)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return outputs, nil
}

func (r *OutputsRepository) Delete(id string) error {
	res, err := r.db.db.Exec(`DELETE FROM outputs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete output %s: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete output %s rows affected: %w", id, err)
	}
	if affected == 0 {
		return fmt.Errorf("output not found: %s", id)
	}
	return nil
}

func (r *OutputsRepository) ListOlderThan(t time.Time) ([]*OutputRecord, error) {
	rows, err := r.db.db.Query(
		`SELECT id, job_id, type, path, mime_type, size_bytes,
		        duration_ms, sample_rate, channels, metadata, created_at
		 FROM outputs WHERE created_at < ? ORDER BY created_at ASC`, t,
	)
	if err != nil {
		return nil, fmt.Errorf("list outputs older than %v: %w", t, err)
	}
	defer rows.Close()

	var outputs []*OutputRecord
	for rows.Next() {
		out := &OutputRecord{}
		if err := rows.Scan(
			&out.ID, &out.JobID, &out.Type, &out.Path, &out.MimeType, &out.SizeBytes,
			&out.DurationMs, &out.SampleRate, &out.Channels, &out.Metadata, &out.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan output: %w", err)
		}
		outputs = append(outputs, out)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return outputs, nil
}

func (r *OutputsRepository) DeleteOlderThan(t time.Time) (int64, error) {
	res, err := r.db.db.Exec(`DELETE FROM outputs WHERE created_at < ?`, t)
	if err != nil {
		return 0, fmt.Errorf("delete outputs older than %v: %w", t, err)
	}
	count, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return count, nil
}
