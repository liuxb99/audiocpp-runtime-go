package api

import "time"

type ErrorResponse struct {
	Error struct {
		Code    string      `json:"code"`
		Message string      `json:"message"`
		Details interface{} `json:"details,omitempty"`
	} `json:"error"`
}

type HealthResponse struct {
	Status        string  `json:"status"`
	Version       string  `json:"version"`
	AudioCppAlive bool    `json:"audiocpp_alive"`
	ModelsCount   int     `json:"models_count"`
	JobsPending   int     `json:"jobs_pending"`
	JobsRunning   int     `json:"jobs_running"`
	UptimeSeconds float64 `json:"uptime_seconds"`
}

type ModelResponse struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Family       string   `json:"family"`
	Task         string   `json:"task"`
	Backend      string   `json:"backend"`
	Capabilities []string `json:"capabilities"`
	Languages    []string `json:"languages"`
	Voices       []string `json:"voices"`
	Size         int64    `json:"size"`
	Format       string   `json:"format"`
}

type TTSRequest struct {
	Model             string            `json:"model"`
	Input             string            `json:"input"`
	Voice             string            `json:"voice,omitempty"`
	VoiceRef          string            `json:"voice_ref,omitempty"`
	ReferenceText     string            `json:"reference_text,omitempty"`
	Language          string            `json:"language,omitempty"`
	Seed              int               `json:"seed,omitempty"`
	Temperature       float64           `json:"temperature,omitempty"`
	TopK              int               `json:"top_k,omitempty"`
	TopP              float64           `json:"top_p,omitempty"`
	MaxTokens         int               `json:"max_tokens,omitempty"`
	RepetitionPenalty float64           `json:"repetition_penalty,omitempty"`
	GuidanceScale     float64           `json:"guidance_scale,omitempty"`
	NumInferenceSteps int               `json:"num_inference_steps,omitempty"`
	Options           map[string]string `json:"options,omitempty"`
	ResponseFormat    string            `json:"response_format,omitempty"`
}

type ASRRequest struct {
	Model    string `json:"model"`
	Audio    string `json:"audio"`
	Language string `json:"language,omitempty"`
	Context  string `json:"text,omitempty"`
	Stream   bool   `json:"stream,omitempty"`
}

type AlignRequest struct {
	Model    string `json:"model"`
	Audio    string `json:"audio"`
	Text     string `json:"text"`
	Language string `json:"language,omitempty"`
}

type CreateJobRequest struct {
	Type     string                 `json:"type"`
	ModelID  string                 `json:"model_id"`
	Request  map[string]interface{} `json:"request"`
	Priority int                    `json:"priority,omitempty"`
}

type JobResponse struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Status      string                 `json:"status"`
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
