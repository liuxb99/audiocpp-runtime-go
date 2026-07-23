package audiocpp

import (
	"context"
	"errors"
	"io"
	"time"

	core "github.com/liuxb99/audiocpp-runtime-go/internal/audiocpp"
	"github.com/liuxb99/audiocpp-runtime-go/internal/backend"
)

// Adapter 包裝 audiocpp.Process + Client 實作 Backend interface
//
// 狀態完全依賴 core.Process.state，不維護自有 state 欄位以消除競態。
type Adapter struct {
	proc   *core.Process
	client *core.Client
	id     string
	caps   []backend.Capability
}

// 編譯期檢查 Adapter 是否實作 Backend
var _ backend.Backend = (*Adapter)(nil)

// New 建立 AudioCpp Adapter
func New(proc *core.Process, client *core.Client, caps ...backend.Capability) *Adapter {
	a := &Adapter{
		proc:   proc,
		client: client,
		id:     "audiocpp",
	}
	if len(caps) > 0 {
		a.caps = make([]backend.Capability, len(caps))
		copy(a.caps, caps)
	}
	return a
}

// ID 回傳後端唯一識別碼
func (a *Adapter) ID() string { return a.id }

// Start 啟動後端進程
func (a *Adapter) Start(ctx context.Context, cfg backend.StartConfig) error {
	if len(cfg.ExtraEnv) > 0 {
		a.proc.ExtraEnv = cfg.ExtraEnv
	}

	if err := a.proc.Start(ctx); err != nil {
		return convertStartError(err)
	}
	return nil
}

// WaitForReady 等待後端就緒
//
// 1. 先等 TCP 層就緒（proc.WaitForReady 成功後設 Process state = Running）
// 2. 再等 HTTP 健康檢查真正可用
// 任一階段失敗則 cleanup child 並回傳型別化錯誤。
func (a *Adapter) WaitForReady(ctx context.Context, timeoutSec int) error {
	// TCP 層就緒
	if err := a.proc.WaitForReady(ctx, time.Duration(timeoutSec)*time.Second); err != nil {
		return convertReadyError(err)
	}

	// HTTP 健康檢查 — 最多重試 3 次，合計不超過 3 秒
	healthCtx, healthCancel := context.WithTimeout(ctx, 3*time.Second)
	defer healthCancel()

	var lastErr error
	for i := 0; i < 3; i++ {
		if _, err := a.client.Health(healthCtx); err == nil {
			// 成功：proc.WaitForReady 已將 Process state 設為 Running
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-healthCtx.Done():
			lastErr = healthCtx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}

	// HTTP 健康全部失敗：cleanup child（proc.Stop 會將 Process state 設為 Stopped）
	a.proc.Stop()
	return backend.NewError(backend.ErrCodeHealthFailed,
		"HTTP health check failed after TCP ready", lastErr)
}

// Health 健康檢查
func (a *Adapter) Health(ctx context.Context) (*backend.Health, error) {
	hr, err := a.client.Health(ctx)
	if err != nil {
		return nil, convertError(err)
	}
	alive := hr.Status == "ok" || hr.Status == "healthy"
	return &backend.Health{
		Status:  hr.Status,
		Backend: hr.Backend,
		Alive:   alive,
	}, nil
}

// Stop 優雅停止
func (a *Adapter) Stop() error {
	if a.proc.StopGraceful() {
		return nil
	}
	// 優雅停止失敗，回退到強制停止
	return a.proc.Stop()
}

// ForceStop 強制停止
func (a *Adapter) ForceStop() error {
	return a.proc.ForceStop()
}

// State 回傳目前狀態
//
// 即時從 Process.state 讀取並做明確 mapping 以消除競態。
func (a *Adapter) State() backend.State {
	return mapProcessState(a.proc.State())
}

// PID 回傳子進程 PID（若無則回傳 -1）
func (a *Adapter) PID() int {
	pid := a.proc.Pid()
	if pid == 0 {
		return -1
	}
	return pid
}

// Alive 回傳後端是否活躍
func (a *Adapter) Alive(ctx context.Context) bool {
	_, err := a.client.Health(ctx)
	return err == nil
}

// Capabilities 回傳後端支援的能力清單
func (a *Adapter) Capabilities() []backend.Capability {
	if a.caps == nil {
		return []backend.Capability{}
	}
	caps := make([]backend.Capability, len(a.caps))
	copy(caps, a.caps)
	return caps
}

// Submit 提交推理請求（ASR/TTS/Task 統一入口）
func (a *Adapter) Submit(ctx context.Context, req *backend.InferenceRequest) (*backend.InferenceResponse, error) {
	switch req.TaskType {
	case "asr":
		return a.submitASR(ctx, req)
	case "tts":
		return a.submitTTS(ctx, req)
	case "task":
		return a.submitTask(ctx, req)
	default:
		return nil, backend.NewError(backend.ErrCodeInferenceFailed,
			"unsupported task type: "+req.TaskType, nil)
	}
}

// Diagnostics 回傳診斷資訊
func (a *Adapter) Diagnostics() backend.Diagnostics {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return backend.Diagnostics{
		State:     a.State(),
		PID:       a.PID(),
		BackendID: a.id,
		Alive:     a.Alive(ctx),
		UptimeMs:  0,
	}
}

// ---------------------------------------------------------------------------
// 內部 Submit 實作
// ---------------------------------------------------------------------------

func (a *Adapter) submitASR(ctx context.Context, req *backend.InferenceRequest) (*backend.InferenceResponse, error) {
	var transResp *core.TranscribeResponse
	var err error

	if req.AudioPath != "" {
		opts := make(map[string]string)
		if req.Options != nil {
			for k, v := range req.Options {
				if s, ok := v.(string); ok {
					opts[k] = s
				}
			}
		}
		transResp, err = a.client.TranscribeMultipart(ctx, req.Model, req.AudioPath, opts)
	} else {
		transReq := &core.TranscribeRequest{
			Model: req.Model,
		}
		if req.Options != nil {
			if lang, ok := req.Options["language"].(string); ok {
				transReq.Language = lang
			}
			if audio, ok := req.Options["audio"].(string); ok {
				transReq.Audio = audio
			}
		}
		transResp, err = a.client.TranscribeJSON(ctx, transReq)
	}
	if err != nil {
		return nil, convertError(err)
	}

	result := make(map[string]interface{})
	if transResp.Timing != nil {
		result["timing"] = transResp.Timing
	}
	return &backend.InferenceResponse{
		Text: transResp.Text,
		Data: result,
	}, nil
}

func (a *Adapter) submitTTS(ctx context.Context, req *backend.InferenceRequest) (*backend.InferenceResponse, error) {
	speechReq := &core.SpeechRequest{
		Model: req.Model,
	}
	if req.Options != nil {
		if input, ok := req.Options["input"].(string); ok {
			speechReq.Input = input
		}
		if voice, ok := req.Options["voice"].(string); ok {
			speechReq.Voice = voice
		}
		if voiceRef, ok := req.Options["voice_ref"].(string); ok {
			speechReq.VoiceRef = voiceRef
		}
		if refText, ok := req.Options["reference_text"].(string); ok {
			speechReq.ReferenceText = refText
		}
		if lang, ok := req.Options["language"].(string); ok {
			speechReq.Language = lang
		}
		if seed, ok := req.Options["seed"].(int); ok {
			speechReq.Seed = seed
		}
		if temp, ok := req.Options["temperature"].(float64); ok {
			speechReq.Temperature = temp
		}
		if topK, ok := req.Options["top_k"].(int); ok {
			speechReq.TopK = topK
		}
		if topP, ok := req.Options["top_p"].(float64); ok {
			speechReq.TopP = topP
		}
		if maxTokens, ok := req.Options["max_tokens"].(int); ok {
			speechReq.MaxTokens = maxTokens
		}
		if format, ok := req.Options["response_format"].(string); ok {
			speechReq.ResponseFormat = format
		}
		if extra, ok := req.Options["options"].(map[string]string); ok {
			speechReq.Options = extra
		}
	}

	resp, err := a.client.Speech(ctx, speechReq)
	if err != nil {
		return nil, convertError(err)
	}
	defer resp.Body.Close()

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, backend.NewError(backend.ErrCodeInferenceFailed,
			"failed to read TTS audio response", err)
	}

	return &backend.InferenceResponse{
		Audio: audioData,
	}, nil
}

func (a *Adapter) submitTask(ctx context.Context, req *backend.InferenceRequest) (*backend.InferenceResponse, error) {
	taskReq := &core.TaskRequest{
		Model:   req.Model,
		Request: req.Options,
	}

	taskResp, err := a.client.RunTask(ctx, taskReq)
	if err != nil {
		return nil, convertError(err)
	}

	result := make(map[string]interface{})
	if taskResp.Text != "" {
		result["text"] = taskResp.Text
	}
	if taskResp.Audio != "" {
		result["audio"] = taskResp.Audio
	}
	if taskResp.SampleRate > 0 {
		result["sample_rate"] = taskResp.SampleRate
	}
	if taskResp.Channels > 0 {
		result["channels"] = taskResp.Channels
	}
	if len(taskResp.NamedAudioOutputs) > 0 {
		result["named_audio_outputs"] = taskResp.NamedAudioOutputs
	}
	if len(taskResp.Segments) > 0 {
		result["segments"] = taskResp.Segments
	}
	if len(taskResp.SpeakerTurns) > 0 {
		result["speaker_turns"] = taskResp.SpeakerTurns
	}
	if len(taskResp.Words) > 0 {
		result["words"] = taskResp.Words
	}
	if taskResp.Timing != nil {
		result["timing"] = taskResp.Timing
	}

	return &backend.InferenceResponse{
		Text: taskResp.Text,
		Data: result,
	}, nil
}

// ---------------------------------------------------------------------------
// 錯誤轉換
// ---------------------------------------------------------------------------

func convertStartError(err error) error {
	if err == nil {
		return nil
	}
	if err.Error() == "already running" {
		return backend.ErrAlreadyStarted
	}
	return backend.NewError(backend.ErrCodeStartFailed, "process start failed", err)
}

func convertReadyError(err error) error {
	if err == nil {
		return nil
	}

	var ae *core.Error
	if errors.As(err, &ae) {
		switch ae.Code {
		case core.ErrServerUnavailable, core.ErrRequestTimeout:
			return backend.ErrReadyTimeout
		}
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return backend.ErrReadyTimeout
	}

	return convertError(err)
}

func convertError(err error) error {
	if err == nil {
		return nil
	}

	var ae *core.Error
	if errors.As(err, &ae) {
		switch ae.Code {
		case core.ErrRequestTimeout:
			return backend.NewError(backend.ErrCodeReadyTimeout, ae.Message, ae)
		case core.ErrServerUnavailable:
			return backend.NewError(backend.ErrCodeHealthFailed, ae.Message, ae)
		case core.ErrModelNotFound:
			return backend.NewError(backend.ErrCodeInferenceFailed, ae.Message, ae)
		case core.ErrInvalidRequest:
			return backend.NewError(backend.ErrCodeInferenceFailed, ae.Message, ae)
		case core.ErrProcessCrash:
			return backend.NewError(backend.ErrCodeStartFailed, ae.Message, ae)
		default:
			return backend.NewError(backend.ErrCodeInferenceFailed, ae.Message, ae)
		}
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return backend.ErrReadyTimeout
	}

	return backend.NewError(backend.ErrCodeInferenceFailed, err.Error(), err)
}

// mapProcessState 將 core.ProcessState 明確映射為 backend.State
func mapProcessState(s core.ProcessState) backend.State {
	switch s {
	case core.StateStopped:
		return backend.StateStopped
	case core.StateStarting:
		return backend.StateStarting
	case core.StateRunning:
		return backend.StateRunning
	case core.StateStopping:
		return backend.StateStopping
	case core.StateCrashed:
		return backend.StateCrashed
	default:
		return backend.StateStopped
	}
}

// NewBuilder 建立一個回傳 Adapter 的工廠函式，供 Runtime 註冊使用
func NewBuilder(proc *core.Process, client *core.Client, caps ...backend.Capability) func() backend.Backend {
	return func() backend.Backend {
		return New(proc, client, caps...)
	}
}
