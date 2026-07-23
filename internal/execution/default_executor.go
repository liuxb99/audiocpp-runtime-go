package execution

import (
	"context"
	"log"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/backend"
)

// DefaultExecutor 為 Executor 介面的預設實作。
//
// 使用 backend.Manager 提交推理請求，並依賴 Mapper 進行請求/回應轉換，
// 以及 Gate 進行執行前的能力檢查。
type DefaultExecutor struct {
	manager *backend.Manager
	mapper  Mapper
	gate    Gate

	// BackendName 用於 Result 中標記執行此請求的後端名稱。
	BackendName string
	// BackendVersion 用於 Result 中標記後端版本。
	BackendVersion string
}

// NewDefaultExecutor 建立一個新的 DefaultExecutor。
//
// manager 用於提交推理請求；mapper 用於 Job 與後端請求間的轉換；
// gate 用於執行前的能力檢查與後端就緒狀態確認。
func NewDefaultExecutor(manager *backend.Manager, mapper Mapper, gate Gate) *DefaultExecutor {
	return &DefaultExecutor{
		manager:        manager,
		mapper:         mapper,
		gate:           gate,
		BackendName:    "audiocpp",
		BackendVersion: "unknown",
	}
}

// Execute 執行一個請求並回傳執行結果。
//
// 實作流程：
//  1. 透過 Mapper 將 Type 轉換為 Capability
//  2. 透過 Gate 檢查後端就緒狀態與能力支援
//  3. 透過 Mapper 將 ExecutionRequest 轉換為 backend.InferenceRequest
//  4. 透過 backend.Manager.Submit 提交推理請求
//  5. 透過 Mapper 將 backend.InferenceResponse 轉換為 Result
//  6. 回傳 Result
func (e *DefaultExecutor) Execute(ctx context.Context, req *ExecutionRequest) (*Result, error) {
	if req == nil {
		return nil, NewError(ErrCodeInvalidRequest, "execution request is nil", nil)
	}

	startedAt := time.Now().UTC()

	// Step 1: 將 Type 映射為 Capability
	capability, err := e.mapper.TaskTypeToCapability(req.Type)
	if err != nil {
		return nil, WrapError(ErrCodeMappingFailed, "failed to map type to capability", err)
	}

	// Step 2: 檢查後端就緒狀態與能力支援
	if err := e.gate.Check(ctx, capability); err != nil {
		return nil, err
	}

	// Step 3: 將 ExecutionRequest 轉換為後端推理請求
	inferenceReq, err := e.mapper.ToInferenceRequest(req)
	if err != nil {
		return nil, WrapError(ErrCodeMappingFailed, "failed to map to inference request", err)
	}

	// Step 4: 提交推理請求
	inferenceResp, err := e.manager.Submit(ctx, inferenceReq)
	if err != nil {
		return nil, WrapError(ErrCodeNoActiveBackend, "backend inference failed", err)
	}

	completedAt := time.Now().UTC()

	// Step 5: 將後端回應轉換為結構化 Result
	result, err := e.mapper.FromInferenceResponse(inferenceResp)
	if err != nil {
		return nil, WrapError(ErrCodeMappingFailed, "failed to map inference response", err)
	}

	// 補齊 Result 中由 DefaultExecutor 負責的欄位
	result.BackendName = e.BackendName
	result.BackendVersion = e.BackendVersion
	result.Model = req.ModelID
	result.Attempt = req.Attempt
	result.StartedAt = startedAt
	result.CompletedAt = completedAt
	result.Duration = completedAt.Sub(startedAt)

	// 從 inferenceResp 中提取 Text/Audio 等額外資料到 Result
	if inferenceResp.Text != "" && result.ErrorCode == "" {
		if req.Type == "asr" {
			log.Printf("[execution] ASR result: text=%q", inferenceResp.Text)
		}
	}

	return result, nil
}
