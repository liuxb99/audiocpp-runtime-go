package execution

import (
	"fmt"

	"github.com/liuxb99/audiocpp-runtime-go/internal/backend"
	"github.com/liuxb99/audiocpp-runtime-go/internal/jobs"
)

// Mapper 定義 Job 與後端請求/回應之間的轉換介面。
//
// Mapper 負責從 Job 中提取 typed 欄位並組裝為 backend.InferenceRequest，
// 以及將 backend.InferenceResponse 轉換為結構化的 Result。
type Mapper interface {
	// ToInferenceRequest 將 Job 轉換為後端推理請求。
	//
	// 應從 job.Request (map[string]interface{}) 中提取 typed 欄位，
	// 並回傳 error 若缺少必要欄位。禁止直接傳遞 map 作為唯一模型。
	ToInferenceRequest(job *jobs.Job) (*backend.InferenceRequest, error)

	// FromInferenceResponse 將後端推理回應轉換為結構化 Result。
	FromInferenceResponse(resp *backend.InferenceResponse) (*Result, error)

	// TaskTypeToCapability 將任務類型映射為後端能力。
	TaskTypeToCapability(taskType jobs.Type) (backend.Capability, error)
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
func (m *DefaultMapper) ToInferenceRequest(job *jobs.Job) (*backend.InferenceRequest, error) {
	if job == nil {
		return nil, NewError(ErrCodeInvalidRequest, "job is nil", nil)
	}

	req := &backend.InferenceRequest{
		Model:   job.ModelID,
		Options: make(map[string]interface{}),
	}

	switch job.Type {
	case jobs.TypeASR:
		req.TaskType = string(InferenceTypeASR)
		// 從 Request 中提取 AudioPath（可選）
		if audioPath, ok := job.Request["audio_path"]; ok {
			if s, ok := audioPath.(string); ok {
				req.AudioPath = s
			}
		}
		// 複製其他 options（排除已知欄位）
		for k, v := range job.Request {
			if k != "audio_path" {
				req.Options[k] = v
			}
		}

	case jobs.TypeTTS:
		req.TaskType = string(InferenceTypeTTS)
		// 必填欄位檢查：input / text
		if _, ok := job.Request["input"]; !ok {
			if _, ok2 := job.Request["text"]; !ok2 {
				return nil, NewError(ErrCodeMissingRequiredField,
					"TTS request missing required field: 'input' or 'text'", nil)
			}
		}
		for k, v := range job.Request {
			req.Options[k] = v
		}

	case jobs.TypeVoiceClone, jobs.TypeVoiceConversion, jobs.TypeSourceSep,
		jobs.TypeMusicGen, jobs.TypeVAD, jobs.TypeDiarization, jobs.TypeAlignment,
		jobs.TypeVoiceDesign:
		req.TaskType = string(InferenceTypeTask)
		// 通用任務：複製所有 request 參數
		for k, v := range job.Request {
			req.Options[k] = v
		}

	default:
		// 未知型別也視為通用任務
		req.TaskType = string(InferenceTypeTask)
		for k, v := range job.Request {
			req.Options[k] = v
		}
	}

	return req, nil
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
func (m *DefaultMapper) TaskTypeToCapability(taskType jobs.Type) (backend.Capability, error) {
	switch taskType {
	case jobs.TypeASR:
		return backend.CapASR, nil
	case jobs.TypeTTS:
		return backend.CapTTS, nil
	case jobs.TypeVoiceClone:
		return backend.CapVoiceClone, nil
	case jobs.TypeVoiceConversion:
		return backend.CapVoiceConversion, nil
	case jobs.TypeSourceSep:
		return backend.CapSourceSeparation, nil
	case jobs.TypeMusicGen:
		return backend.CapMusicGeneration, nil
	case jobs.TypeVAD:
		return backend.CapVAD, nil
	case jobs.TypeDiarization:
		return backend.CapDiarization, nil
	case jobs.TypeAlignment:
		return backend.CapAlignment, nil
	case jobs.TypeVoiceDesign:
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
