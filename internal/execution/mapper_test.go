package execution

import (
	"testing"

	"github.com/liuxb99/audiocpp-runtime-go/internal/backend"
	"github.com/liuxb99/audiocpp-runtime-go/internal/jobs"
)

func TestMapper_ASR_JobMapping(t *testing.T) {
	m := NewDefaultMapper()
	job := &jobs.Job{
		ID:      "asr-1",
		Type:    jobs.TypeASR,
		ModelID: "whisper-1",
		Request: map[string]interface{}{
			"audio_path": "/audio/test.wav",
			"language":   "en",
		},
	}

	req, err := m.ToInferenceRequest(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.TaskType != "asr" {
		t.Errorf("expected TaskType 'asr', got %q", req.TaskType)
	}
	if req.Model != "whisper-1" {
		t.Errorf("expected Model 'whisper-1', got %q", req.Model)
	}
	if req.AudioPath != "/audio/test.wav" {
		t.Errorf("expected AudioPath '/audio/test.wav', got %q", req.AudioPath)
	}
	if req.Options["language"] != "en" {
		t.Errorf("expected Options.language 'en', got %v", req.Options["language"])
	}
	// audio_path should not be in Options
	if _, ok := req.Options["audio_path"]; ok {
		t.Error("audio_path should not be in Options")
	}
}

func TestMapper_ASR_NoAudioPath(t *testing.T) {
	m := NewDefaultMapper()
	job := &jobs.Job{
		ID:      "asr-2",
		Type:    jobs.TypeASR,
		ModelID: "whisper-1",
		Request: map[string]interface{}{},
	}

	req, err := m.ToInferenceRequest(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.TaskType != "asr" {
		t.Errorf("expected TaskType 'asr', got %q", req.TaskType)
	}
	if req.AudioPath != "" {
		t.Errorf("expected empty AudioPath, got %q", req.AudioPath)
	}
}

func TestMapper_TTS_JobMapping(t *testing.T) {
	m := NewDefaultMapper()
	job := &jobs.Job{
		ID:      "tts-1",
		Type:    jobs.TypeTTS,
		ModelID: "tts-model-1",
		Request: map[string]interface{}{
			"input": "Hello world",
			"voice": "alba",
		},
	}

	req, err := m.ToInferenceRequest(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.TaskType != "tts" {
		t.Errorf("expected TaskType 'tts', got %q", req.TaskType)
	}
	if req.Model != "tts-model-1" {
		t.Errorf("expected Model 'tts-model-1', got %q", req.Model)
	}
	if req.Options["input"] != "Hello world" {
		t.Errorf("expected Options.input 'Hello world', got %v", req.Options["input"])
	}
	if req.Options["voice"] != "alba" {
		t.Errorf("expected Options.voice 'alba', got %v", req.Options["voice"])
	}
}

func TestMapper_TTS_WithTextField(t *testing.T) {
	m := NewDefaultMapper()
	job := &jobs.Job{
		ID:      "tts-2",
		Type:    jobs.TypeTTS,
		ModelID: "tts-model-1",
		Request: map[string]interface{}{
			"text": "Hello via text field",
		},
	}

	req, err := m.ToInferenceRequest(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.TaskType != "tts" {
		t.Errorf("expected TaskType 'tts', got %q", req.TaskType)
	}
	if req.Options["text"] != "Hello via text field" {
		t.Errorf("expected Options.text 'Hello via text field', got %v", req.Options["text"])
	}
}

func TestMapper_GenericTask_Mapping(t *testing.T) {
	m := NewDefaultMapper()
	job := &jobs.Job{
		ID:      "task-1",
		Type:    jobs.TypeVoiceClone,
		ModelID: "vc-model",
		Request: map[string]interface{}{
			"source_audio": "/audio/source.wav",
			"target_text":  "test",
		},
	}

	req, err := m.ToInferenceRequest(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.TaskType != "task" {
		t.Errorf("expected TaskType 'task', got %q", req.TaskType)
	}
	if req.Options["source_audio"] != "/audio/source.wav" {
		t.Errorf("expected Options.source_audio, got %v", req.Options["source_audio"])
	}
	if req.Options["target_text"] != "test" {
		t.Errorf("expected Options.target_text, got %v", req.Options["target_text"])
	}
}

func TestMapper_TTS_MissingInput(t *testing.T) {
	m := NewDefaultMapper()
	job := &jobs.Job{
		ID:      "tts-bad",
		Type:    jobs.TypeTTS,
		ModelID: "tts-model",
		Request: map[string]interface{}{
			"voice": "alba",
		},
	}

	_, err := m.ToInferenceRequest(job)
	if err == nil {
		t.Fatal("expected error for missing input/text field")
	}

	var execErr *Error
	if !IsExecutionError(err) {
		t.Errorf("expected ExecutionError, got %T", err)
	} else {
		execErr = err.(*Error)
		if execErr.Code != ErrCodeMissingRequiredField {
			t.Errorf("expected error code %q, got %q", ErrCodeMissingRequiredField, execErr.Code)
		}
	}
}

func TestMapper_FromInferenceResponse(t *testing.T) {
	m := NewDefaultMapper()
	resp := &backend.InferenceResponse{
		Text: "hello world",
		Data: map[string]interface{}{
			"output_ref":    "/outputs/result.wav",
			"error_code":    "",
			"error_message": "",
		},
	}

	result, err := m.FromInferenceResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.OutputRef != "/outputs/result.wav" {
		t.Errorf("expected OutputRef '/outputs/result.wav', got %q", result.OutputRef)
	}
	if result.ErrorCode != "" {
		t.Errorf("expected empty ErrorCode, got %q", result.ErrorCode)
	}
	if result.ErrorMessage != "" {
		t.Errorf("expected empty ErrorMessage, got %q", result.ErrorMessage)
	}
}

func TestMapper_FromInferenceResponse_WithError(t *testing.T) {
	m := NewDefaultMapper()
	resp := &backend.InferenceResponse{
		Data: map[string]interface{}{
			"error_code":    "INFERENCE_FAILED",
			"error_message": "model failed to process",
		},
	}

	result, err := m.FromInferenceResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ErrorCode != "INFERENCE_FAILED" {
		t.Errorf("expected ErrorCode 'INFERENCE_FAILED', got %q", result.ErrorCode)
	}
	if result.ErrorMessage != "model failed to process" {
		t.Errorf("expected ErrorMessage 'model failed to process', got %q", result.ErrorMessage)
	}
}

func TestMapper_FromInferenceResponse_Nil(t *testing.T) {
	m := NewDefaultMapper()
	_, err := m.FromInferenceResponse(nil)
	if err == nil {
		t.Fatal("expected error for nil response")
	}
}

func TestMapper_TaskTypeToCapability(t *testing.T) {
	m := NewDefaultMapper()

	tests := []struct {
		jobType     jobs.Type
		expectedCap backend.Capability
		expectError bool
	}{
		{jobs.TypeASR, backend.CapASR, false},
		{jobs.TypeTTS, backend.CapTTS, false},
		{jobs.TypeVoiceClone, backend.CapVoiceClone, false},
		{jobs.TypeVoiceConversion, backend.CapVoiceConversion, false},
		{jobs.TypeSourceSep, backend.CapSourceSeparation, false},
		{jobs.TypeMusicGen, backend.CapMusicGeneration, false},
		{jobs.TypeVAD, backend.CapVAD, false},
		{jobs.TypeDiarization, backend.CapDiarization, false},
		{jobs.TypeAlignment, backend.CapAlignment, false},
		{jobs.TypeVoiceDesign, backend.CapVoiceDesign, false},
		{jobs.Type("unknown"), "", true},
	}

	for _, tt := range tests {
		cap, err := m.TaskTypeToCapability(tt.jobType)
		if tt.expectError {
			if err == nil {
				t.Errorf("expected error for type %q", tt.jobType)
			}
			continue
		}
		if err != nil {
			t.Errorf("unexpected error for type %q: %v", tt.jobType, err)
			continue
		}
		if cap != tt.expectedCap {
			t.Errorf("expected capability %q for type %q, got %q", tt.expectedCap, tt.jobType, cap)
		}
	}
}

func TestMapper_ToInferenceRequest_NilJob(t *testing.T) {
	m := NewDefaultMapper()
	_, err := m.ToInferenceRequest(nil)
	if err == nil {
		t.Fatal("expected error for nil job")
	}
}
