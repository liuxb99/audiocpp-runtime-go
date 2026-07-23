package backend

// State 後端生命週期狀態
type State int32

const (
	StateStopped  State = 0
	StateStarting State = 1
	StateRunning  State = 2
	StateStopping State = 3
	StateCrashed  State = 4
)

func (s State) String() string {
	switch s {
	case StateStopped:
		return "stopped"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateCrashed:
		return "crashed"
	default:
		return "unknown"
	}
}

// Capability 後端能力
type Capability string

const (
	CapTTS              Capability = "tts"
	CapASR              Capability = "asr"
	CapVoiceClone       Capability = "voice_clone"
	CapVoiceConversion  Capability = "voice_conversion"
	CapSourceSeparation Capability = "source_separation"
	CapMusicGeneration  Capability = "music_generation"
	CapVAD              Capability = "vad"
	CapDiarization      Capability = "diarization"
	CapAlignment        Capability = "alignment"
	CapVoiceDesign      Capability = "voice_design"
)

// Health 健康檢查結果
type Health struct {
	Status  string `json:"status"`
	Backend string `json:"backend"`
	Alive   bool   `json:"alive"`
}

// StartConfig 後端啟動配置
type StartConfig struct {
	Device   int
	Threads  int
	LazyLoad bool
	ExtraEnv []string
}

// InferenceRequest 推理請求
type InferenceRequest struct {
	Model     string
	TaskType  string // "asr", "tts", "task"
	AudioPath string // for ASR multipart
	Options   map[string]interface{}
}

// InferenceResponse 推理回應
type InferenceResponse struct {
	Data  map[string]interface{}
	Audio []byte // for TTS
	Text  string // for ASR
}

// Diagnostics 診斷資訊
type Diagnostics struct {
	State     State  `json:"state"`
	PID       int    `json:"pid"`
	BackendID string `json:"backend_id"`
	Alive     bool   `json:"alive"`
	UptimeMs  int64  `json:"uptime_ms"`
}
