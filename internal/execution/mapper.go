package execution

import (
	"fmt"

	"github.com/liuxb99/audiocpp-runtime-go/internal/backend"
)

// Mapper 定義 Job 與後端請求/回應之間的轉換介面。
//
// Mapper 負責從 Job 中提取 typed 欄位並組裝為 backend.InferenceRequest，
// 以及將 backend.InferenceResponse 轉換為結構化的 Result。
type Mapper interface {
	// ToInferenceRequest 將 Job 轉換為後端推理請求。
	//
	// 應從 req.Request (map[string]interface{}) 中提取 typed 欄位，
	// 並回傳 error 若缺少必要欄位。禁止直接傳遞 map 作為唯一模型。
	ToInferenceRequest(req *ExecutionRequest) (*backend.InferenceRequest, error)

	// FromInferenceResponse 將後端推理回應轉換為結構化 Result。
	FromInferenceResponse(resp *backend.InferenceResponse) (*Result, error)

	// TaskTypeToCapability 將任務類型映射為後端能力。
	TaskTypeToCapability(taskType string) (backend.Capability, error)
}

// InferenceType 為後端推理請求類型的字串常量。
//
// 對應 backend.InferenceRequest.TaskType 欄位值。
type InferenceType string

const (
	// InferenceTypeASR 語音辨識請求類型。
	InferenceTypeASR InferenceType = "asr"
	// InferenceTypeTTS 語音合成請求類型。
	InferenceTypeTTS InferenceType = "tts"
	// InferenceTypeTask 通用任務請求類型。
	InferenceTypeTask InferenceType = "task"
)

// DefaultMapper 為 Mapper 介面的預設實作。
//
// 支援 ASR、TTS 與通用任務（Generic Task）三種 Job 類型。
type DefaultMapper struct{}

// NewDefaultMapper 建立一個預設 Mapper。
func NewDefaultMapper() *DefaultMapper {
	return &DefaultMapper{}
}

// ToInferenceRequest 將 Job 轉換為 backend.InferenceRequest。
//
// 從 job.Request map 中提取必要欄位；若缺少必填欄位則回傳 error。
func (m *DefaultMapper) ToInferenceRequest(req *ExecutionRequest) (*backend.InferenceRequest, error) {
	if req == nil {
		return nil, NewError(ErrCodeInvalidRequest, "execution request is nil", nil)
	}

	ir := &backend.InferenceRequest{
		Model:   req.ModelID,
		Options: make(map[string]interface{}),
	}

	switch req.Type {
	case string(InferenceTypeASR):
		ir.TaskType = string(InferenceTypeASR)
		// 從 Request 中提取 AudioPath（可選）
		if audioPath, ok := req.Request["audio_path"]; ok {
			if s, ok := audioPath.(string); ok {
				ir.AudioPath = s
			}
		}
		// 複製其他 options（排除已知欄位）
		for k, v := range req.Request {
			if k != "audio_path" {
				ir.Options[k] = v
			}
		}

	case string(InferenceTypeTTS):
		ir.TaskType = string(InferenceTypeTTS)
		// 必填欄位檢查：input / text
		if _, ok := req.Request["input"]; !ok {
			if _, ok2 := req.Request["text"]; !ok2 {
				return nil, NewError(ErrCodeMissingRequiredField,
					"TTS request missing required field: 'input' or 'text'", nil)
			}
		}
		for k, v := range req.Request {
			ir.Options[k] = v
		}

	default:
		// 其他型別（voice_clone, voice_conversion, task 等）視為通用任務
		ir.TaskType = string(InferenceTypeTask)
		for k, v := range req.Request {
			ir.Options[k] = v
		}
	}

	return ir, nil
}

// FromInferenceResponse 將後端回應轉換為結構化 Result。
//
// 注意：此方法只能填寫可從 InferenceResponse 取得的欄位，
// 其餘欄位（如 BackendName、BackendVersion、Attempt 等）應由呼叫者補齊。
func (m *DefaultMapper) FromInferenceResponse(resp *backend.InferenceResponse) (*Result, error) {
	if resp == nil {
		return nil, NewError(ErrCodeInvalidRequest, "inference response is nil", nil)
	}

	result := &Result{
		// 從 Data map 中提取可能的欄位
		OutputRef: extractString(resp.Data, "output_ref"),
	}

	// 若 Data 中有 "error_code" 或 "error_message"，表示後端層級錯誤
	if code, ok := resp.Data["error_code"]; ok {
		if s, ok := code.(string); ok && s != "" {
			result.ErrorCode = s
		}
	}
	if msg, ok := resp.Data["error_message"]; ok {
		if s, ok := msg.(string); ok && s != "" {
			result.ErrorMessage = s
		}
	}

	return result, nil
}

// TaskTypeToCapability 將 jobs.Type 映射為 backend.Capability。
func (m *DefaultMapper) TaskTypeToCapability(taskType string) (backend.Capability, error) {
	switch taskType {
	case "asr":
		return backend.CapASR, nil
	case "tts":
		return backend.CapTTS, nil
	case "voice_clone":
		return backend.CapVoiceClone, nil
	case "voice_conversion":
		return backend.CapVoiceConversion, nil
	case "source_separation":
		return backend.CapSourceSeparation, nil
	case "music_generation":
		return backend.CapMusicGeneration, nil
	case "vad":
		return backend.CapVAD, nil
	case "diarization":
		return backend.CapDiarization, nil
	case "alignment":
		return backend.CapAlignment, nil
	case "voice_design":
		return backend.CapVoiceDesign, nil
	default:
		return "", NewError(ErrCodeMappingFailed,
			fmt.Sprintf("unknown job type: %s", taskType), nil)
	}
}

// extractString 從 map 中安全提取 string 值。
func extractString(data map[string]interface{}, key string) string {
	if data == nil {
		return ""
	}
	v, ok := data[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
