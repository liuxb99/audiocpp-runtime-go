package jobs

import (
	"encoding/json"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/storage"
)

type Type string

const (
	TypeTTS             Type = "tts"
	TypeASR             Type = "asr"
	TypeVoiceClone      Type = "voice_clone"
	TypeVoiceConversion Type = "voice_conversion"
	TypeSourceSep       Type = "source_separation"
	TypeMusicGen        Type = "music_generation"
	TypeVAD             Type = "vad"
	TypeDiarization     Type = "diarization"
	TypeAlignment       Type = "alignment"
	TypeVoiceDesign     Type = "voice_design"
)

var validTypes = map[Type]bool{
	TypeTTS:             true,
	TypeASR:             true,
	TypeVoiceClone:      true,
	TypeVoiceConversion: true,
	TypeSourceSep:       true,
	TypeMusicGen:        true,
	TypeVAD:             true,
	TypeDiarization:     true,
	TypeAlignment:       true,
	TypeVoiceDesign:     true,
}

func (t Type) IsValid() bool {
	return validTypes[t]
}

type Status string

// Backward-compatible aliases
const (
	StatusCompleted Status = "succeeded"
	StatusCancelled Status = "canceled"
)

type Job struct {
	ID          string                 `json:"id"`
	Type        Type                   `json:"type"`
	Status      Status                 `json:"status"`
	ModelID     string                 `json:"model_id"`
	Request     map[string]interface{} `json:"request"`
	Result      map[string]interface{} `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Progress    float64                `json:"progress"`
	CreatedAt   time.Time              `json:"created_at"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	Priority    int                    `json:"priority"`

	// S6 — Job Ownership / Lease
	WorkerID   string     `json:"worker_id,omitempty"`
	ClaimedAt  *time.Time `json:"claimed_at,omitempty"`
	LeaseUntil *time.Time `json:"lease_until,omitempty"`
	Attempt    int        `json:"attempt"`

	// S11 — Result persistence
	BackendName    string `json:"backend_name,omitempty"`
	BackendVersion string `json:"backend_version,omitempty"`
	TraceID        string `json:"trace_id,omitempty"`
	OutputRef      string `json:"output_ref,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
}

func (j *Job) ToRecord() *storage.JobRecord {
	reqJSON, _ := json.Marshal(j.Request)
	reqStr := string(reqJSON)

	var resultStr *string
	if j.Result != nil {
		resJSON, _ := json.Marshal(j.Result)
		s := string(resJSON)
		resultStr = &s
	}

	var errStr *string
	if j.Error != "" {
		e := j.Error
		errStr = &e
	}

	var durationMs *int64
	if j.StartedAt != nil && j.CompletedAt != nil && !j.StartedAt.IsZero() && !j.CompletedAt.IsZero() {
		d := j.CompletedAt.Sub(*j.StartedAt).Milliseconds()
		durationMs = &d
	}

	return &storage.JobRecord{
		ID:          j.ID,
		Type:        string(j.Type),
		Status:      string(j.Status),
		ModelID:     j.ModelID,
		Request:     reqStr,
		Result:      resultStr,
		Error:       errStr,
		Progress:    j.Progress,
		CreatedAt:   j.CreatedAt,
		StartedAt:   j.StartedAt,
		CompletedAt: j.CompletedAt,
		Priority:    j.Priority,

		WorkerID:   j.WorkerID,
		ClaimedAt:  j.ClaimedAt,
		LeaseUntil: j.LeaseUntil,
		Attempt:    j.Attempt,

		BackendName:    j.BackendName,
		BackendVersion: j.BackendVersion,
		TraceID:        j.TraceID,
		OutputRef:      j.OutputRef,
		ErrorCode:      j.ErrorCode,
		ErrorMessage:   j.ErrorMessage,
		DurationMs:     durationMs,
	}
}

func JobFromRecord(r *storage.JobRecord) *Job {
	req := make(map[string]interface{})
	if err := json.Unmarshal([]byte(r.Request), &req); err != nil {
		req = make(map[string]interface{})
	}

	job := &Job{
		ID:          r.ID,
		Type:        Type(r.Type),
		Status:      Status(r.Status),
		ModelID:     r.ModelID,
		Request:     req,
		Progress:    r.Progress,
		CreatedAt:   r.CreatedAt,
		StartedAt:   r.StartedAt,
		CompletedAt: r.CompletedAt,
		Priority:    r.Priority,

		WorkerID:   r.WorkerID,
		ClaimedAt:  r.ClaimedAt,
		LeaseUntil: r.LeaseUntil,
		Attempt:    r.Attempt,

		BackendName:    r.BackendName,
		BackendVersion: r.BackendVersion,
		TraceID:        r.TraceID,
		OutputRef:      r.OutputRef,
		ErrorCode:      r.ErrorCode,
		ErrorMessage:   r.ErrorMessage,
	}

	if r.Result != nil {
		res := make(map[string]interface{})
		if err := json.Unmarshal([]byte(*r.Result), &res); err == nil {
			job.Result = res
		}
	}

	if r.Error != nil {
		job.Error = *r.Error
	}

	return job
}
