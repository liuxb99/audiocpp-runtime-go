package storage

import (
	"database/sql"
	"fmt"
	"time"
)

type JobRecord struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Status      string     `json:"status"`
	ModelID     string     `json:"model_id"`
	Request     string     `json:"request"`
	Result      *string    `json:"result,omitempty"`
	Error       *string    `json:"error,omitempty"`
	Progress    float64    `json:"progress"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Priority    int        `json:"priority"`
}

type JobsRepository struct {
	db *DB
}

func NewJobsRepository(db *DB) *JobsRepository {
	return &JobsRepository{db: db}
}

func (r *JobsRepository) Create(job *JobRecord) error {
	_, err := r.db.db.Exec(
		`INSERT INTO jobs (id, type, status, model_id, request, result, error, progress, created_at, started_at, completed_at, priority)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Type, job.Status, job.ModelID, job.Request,
		job.Result, job.Error, job.Progress, job.CreatedAt,
		job.StartedAt, job.CompletedAt, job.Priority,
	)
	if err != nil {
		return fmt.Errorf("create job: %w", err)
	}
	return nil
}

func (r *JobsRepository) Get(id string) (*JobRecord, error) {
	row := r.db.db.QueryRow(
		`SELECT id, type, status, model_id, request, result, error, progress,
		        created_at, started_at, completed_at, priority
		 FROM jobs WHERE id = ?`, id,
	)

	job := &JobRecord{}
	err := row.Scan(
		&job.ID, &job.Type, &job.Status, &job.ModelID, &job.Request,
		&job.Result, &job.Error, &job.Progress,
		&job.CreatedAt, &job.StartedAt, &job.CompletedAt, &job.Priority,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("job not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get job %s: %w", id, err)
	}
	return job, nil
}

func (r *JobsRepository) List(limit, offset int, status string) ([]*JobRecord, error) {
	var rows *sql.Rows
	var err error

	if status != "" {
		rows, err = r.db.db.Query(
			`SELECT id, type, status, model_id, request, result, error, progress,
			        created_at, started_at, completed_at, priority
			 FROM jobs WHERE status = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
			status, limit, offset,
		)
	} else {
		rows, err = r.db.db.Query(
			`SELECT id, type, status, model_id, request, result, error, progress,
			        created_at, started_at, completed_at, priority
			 FROM jobs ORDER BY created_at DESC LIMIT ? OFFSET ?`,
			limit, offset,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*JobRecord
	for rows.Next() {
		job := &JobRecord{}
		if err := rows.Scan(
			&job.ID, &job.Type, &job.Status, &job.ModelID, &job.Request,
			&job.Result, &job.Error, &job.Progress,
			&job.CreatedAt, &job.StartedAt, &job.CompletedAt, &job.Priority,
		); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return jobs, nil
}

func (r *JobsRepository) Update(job *JobRecord) error {
	res, err := r.db.db.Exec(
		`UPDATE jobs SET type=?, status=?, model_id=?, request=?, result=?, error=?,
		                progress=?, started_at=?, completed_at=?, priority=?
		 WHERE id=?`,
		job.Type, job.Status, job.ModelID, job.Request, job.Result, job.Error,
		job.Progress, job.StartedAt, job.CompletedAt, job.Priority, job.ID,
	)
	if err != nil {
		return fmt.Errorf("update job %s: %w", job.ID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update job %s rows affected: %w", job.ID, err)
	}
	if affected == 0 {
		return fmt.Errorf("job not found: %s", job.ID)
	}
	return nil
}

func (r *JobsRepository) Delete(id string) error {
	res, err := r.db.db.Exec(`DELETE FROM jobs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete job %s: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete job %s rows affected: %w", id, err)
	}
	if affected == 0 {
		return fmt.Errorf("job not found: %s", id)
	}
	return nil
}

func (r *JobsRepository) CountByStatus() (map[string]int, error) {
	rows, err := r.db.db.Query(`SELECT status, COUNT(*) FROM jobs GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("count by status: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan count: %w", err)
		}
		counts[status] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return counts, nil
}

func (r *JobsRepository) ListPending(limit int) ([]*JobRecord, error) {
	rows, err := r.db.db.Query(
		`SELECT id, type, status, model_id, request, result, error, progress,
		        created_at, started_at, completed_at, priority
		 FROM jobs WHERE status = 'pending'
		 ORDER BY priority DESC, created_at ASC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list pending jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*JobRecord
	for rows.Next() {
		job := &JobRecord{}
		if err := rows.Scan(
			&job.ID, &job.Type, &job.Status, &job.ModelID, &job.Request,
			&job.Result, &job.Error, &job.Progress,
			&job.CreatedAt, &job.StartedAt, &job.CompletedAt, &job.Priority,
		); err != nil {
			return nil, fmt.Errorf("scan pending job: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return jobs, nil
}
