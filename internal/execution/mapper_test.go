package execution

import (
	"testing"

	"github.com/liuxb99/audiocpp-runtime-go/internal/backend"
)

func TestMapper_ASR_JobMapping(t *testing.T) {
	m := NewDefaultMapper()
	req := &ExecutionRequest{
		JobID:   "asr-1",
		Type:    "asr",
		ModelID: "whisper-1",
		Request: map[string]interface{}{
			"audio_path": "/audio/test.wav",
			"language":   "en",
		},
	}

	ir, err := m.ToInferenceRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ir.TaskType != "asr" {
		t.Errorf("expected TaskType 'asr', got %q", ir.TaskType)
	}
	if ir.Model != "whisper-1" {
		t.Errorf("expected Model 'whisper-1', got %q", ir.Model)
	}
	if ir.AudioPath != "/audio/test.wav" {
		t.Errorf("expected AudioPath '/audio/test.wav', got %q", ir.AudioPath)
	}
	if ir.Options["language"] != "en" {
		t.Errorf("expected Options.language 'en', got %v", ir.Options["language"])
	}
	// audio_path should not be in Options
	if _, ok := ir.Options["audio_path"]; ok {
		t.Error("audio_path should not be in Options")
	}
}

func TestMapper_ASR_NoAudioPath(t *testing.T) {
	m := NewDefaultMapper()
	req := &ExecutionRequest{
		JobID:   "asr-2",
		Type:    "asr",
		ModelID: "whisper-1",
		Request: map[string]interface{}{},
	}

	ir, err := m.ToInferenceRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ir.TaskType != "asr" {
		t.Errorf("expected TaskType 'asr', got %q", ir.TaskType)
	}
	if ir.AudioPath != "" {
		t.Errorf("expected empty AudioPath, got %q", ir.AudioPath)
	}
}

func TestMapper_TTS_JobMapping(t *testing.T) {
	m := NewDefaultMapper()
	req := &ExecutionRequest{
		JobID:   "tts-1",
		Type:    "tts",
		ModelID: "tts-model-1",
		Request: map[string]interface{}{
			"input": "Hello world",
			"voice": "alba",
		},
	}

	ir, err := m.ToInferenceRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ir.TaskType != "tts" {
		t.Errorf("expected TaskType 'tts', got %q", ir.TaskType)
	}
	if ir.Model != "tts-model-1" {
		t.Errorf("expected Model 'tts-model-1', got %q", ir.Model)
	}
	if ir.Options["input"] != "Hello world" {
		t.Errorf("expected Options.input 'Hello world', got %v", ir.Options["input"])
	}
	if ir.Options["voice"] != "alba" {
		t.Errorf("expected Options.voice 'alba', got %v", ir.Options["voice"])
	}
}

func TestMapper_TTS_WithTextField(t *testing.T) {
	m := NewDefaultMapper()
	req := &ExecutionRequest{
		JobID:   "tts-2",
		Type:    "tts",
		ModelID: "tts-model-1",
		Request: map[string]interface{}{
			"text": "Hello via text field",
		},
	}

	ir, err := m.ToInferenceRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ir.TaskType != "tts" {
		t.Errorf("expected TaskType 'tts', got %q", ir.TaskType)
	}
	if ir.Options["text"] != "Hello via text field" {
		t.Errorf("expected Options.text 'Hello via text field', got %v", ir.Options["text"])
	}
}

func TestMapper_GenericTask_Mapping(t *testing.T) {
	m := NewDefaultMapper()
	req := &ExecutionRequest{
		JobID:   "task-1",
		Type:    "voice_clone",
		ModelID: "vc-model",
		Request: map[string]interface{}{
			"source_audio": "/audio/source.wav",
			"target_text":  "test",
		},
	}

	ir, err := m.ToInferenceRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ir.TaskType != "task" {
		t.Errorf("expected TaskType 'task', got %q", ir.TaskType)
	}
	if ir.Options["source_audio"] != "/audio/source.wav" {
		t.Errorf("expected Options.source_audio, got %v", ir.Options["source_audio"])
	}
	if ir.Options["target_text"] != "test" {
		t.Errorf("expected Options.target_text, got %v", ir.Options["target_text"])
	}
}

func TestMapper_TTS_MissingInput(t *testing.T) {
	m := NewDefaultMapper()
	req := &ExecutionRequest{
		JobID:   "tts-bad",
		Type:    "tts",
		ModelID: "tts-model",
		Request: map[string]interface{}{
			"voice": "alba",
		},
	}

	_, err := m.ToInferenceRequest(req)
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
		taskType    string
		expectedCap backend.Capability
		expectError bool
	}{
		{"asr", backend.CapASR, false},
		{"tts", backend.CapTTS, false},
		{"voice_clone", backend.CapVoiceClone, false},
		{"voice_conversion", backend.CapVoiceConversion, false},
		{"source_separation", backend.CapSourceSeparation, false},
		{"music_generation", backend.CapMusicGeneration, false},
		{"vad", backend.CapVAD, false},
		{"diarization", backend.CapDiarization, false},
		{"alignment", backend.CapAlignment, false},
		{"voice_design", backend.CapVoiceDesign, false},
		{"unknown", "", true},
	}

	for _, tt := range tests {
		cap, err := m.TaskTypeToCapability(tt.taskType)
		if tt.expectError {
			if err == nil {
				t.Errorf("expected error for type %q", tt.taskType)
			}
			continue
		}
		if err != nil {
			t.Errorf("unexpected error for type %q: %v", tt.taskType, err)
			continue
		}
		if cap != tt.expectedCap {
			t.Errorf("expected capability %q for type %q, got %q", tt.expectedCap, tt.taskType, cap)
		}
	}
}

func TestMapper_ToInferenceRequest_Nil(t *testing.T) {
	m := NewDefaultMapper()
	_, err := m.ToInferenceRequest(nil)
	if err == nil {
		t.Fatal("expected error for nil request")
	}
}
