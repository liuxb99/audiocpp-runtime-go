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

const (
	StatusPending   Status = "pending"
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

var validStatuses = map[Status]bool{
	StatusPending:   true,
	StatusQueued:    true,
	StatusRunning:   true,
	StatusCompleted: true,
	StatusFailed:    true,
	StatusCancelled: true,
}

var terminalStatuses = map[Status]bool{
	StatusCompleted: true,
	StatusFailed:    true,
	StatusCancelled: true,
}

func (s Status) IsValid() bool {
	return validStatuses[s]
}

func (s Status) IsTerminal() bool {
	return terminalStatuses[s]
}

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
