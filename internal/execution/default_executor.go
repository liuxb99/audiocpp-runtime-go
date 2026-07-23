package execution

import (
	"context"
	"log"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/backend"
	"github.com/liuxb99/audiocpp-runtime-go/internal/jobs"
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

// Execute 執行一個 Job 並回傳執行結果。
//
// 實作流程：
//  1. 透過 Mapper 將 Job 的 Type 轉換為 Capability
//  2. 透過 Gate 檢查後端就緒狀態與能力支援
//  3. 透過 Mapper 將 Job 轉換為 backend.InferenceRequest
//  4. 透過 backend.Manager.Submit 提交推理請求
//  5. 透過 Mapper 將 backend.InferenceResponse 轉換為 Result
//  6. 回傳 Result
func (e *DefaultExecutor) Execute(ctx context.Context, job *jobs.Job) (*Result, error) {
	if job == nil {
		return nil, NewError(ErrCodeInvalidRequest, "job is nil", nil)
	}

	startedAt := time.Now().UTC()

	// Step 1: 將 Job Type 映射為 Capability
	capability, err := e.mapper.TaskTypeToCapability(job.Type)
	if err != nil {
		return nil, WrapError(ErrCodeMappingFailed, "failed to map job type to capability", err)
	}

	// Step 2: 檢查後端就緒狀態與能力支援
	if err := e.gate.Check(ctx, capability); err != nil {
		return nil, err
	}

	// Step 3: 將 Job 轉換為後端推理請求
	inferenceReq, err := e.mapper.ToInferenceRequest(job)
	if err != nil {
		return nil, WrapError(ErrCodeMappingFailed, "failed to map job to inference request", err)
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
	result.Model = job.ModelID
	result.Attempt = job.Attempt
	result.StartedAt = startedAt
	result.CompletedAt = completedAt
	result.Duration = completedAt.Sub(startedAt)

	// 從 inferenceResp 中提取 Text/Audio 等額外資料到 Result
	// 這些資訊會被 Mapper 的 FromInferenceResponse 使用
	if inferenceResp.Text != "" && result.ErrorCode == "" {
		// 保留 Text 給後續的 Job 結果處理
		if job.Type == jobs.TypeASR {
			if result.ErrorCode == "" {
				log.Printf("[execution] ASR result: text=%q", inferenceResp.Text)
			}
		}
	}

	return result, nil
}

// ResultToMap 將 execution.Result 轉換為 map[string]interface{}，
// 用於儲存在 Job.Result 欄位中。
func ResultToMap(r *Result) map[string]interface{} {
	if r == nil {
		return nil
	}
	m := make(map[string]interface{})
	m["backend_name"] = r.BackendName
	m["backend_version"] = r.BackendVersion
	m["model"] = r.Model
	m["attempt"] = r.Attempt
	m["started_at"] = r.StartedAt.Format(time.RFC3339Nano)
	m["completed_at"] = r.CompletedAt.Format(time.RFC3339Nano)
	m["duration_ms"] = r.Duration.Milliseconds()
	m["trace_id"] = r.TraceID
	m["output_ref"] = r.OutputRef
	if r.ErrorCode != "" {
		m["error_code"] = r.ErrorCode
	}
	if r.ErrorMessage != "" {
		m["error_message"] = r.ErrorMessage
	}
	return m
}

// ApplyResultToJob 將 execution.Result 的結構化欄位同步到 Job 的對應欄位。
func ApplyResultToJob(job *jobs.Job, r *Result) {
	if job == nil || r == nil {
		return
	}
	job.BackendName = r.BackendName
	job.BackendVersion = r.BackendVersion
	job.TraceID = r.TraceID
	job.OutputRef = r.OutputRef
	job.ErrorCode = r.ErrorCode
	job.ErrorMessage = r.ErrorMessage
	if r.ErrorCode != "" {
		job.Error = r.ErrorMessage
	}
}

// ToJobResult 將 execution.Result 轉換為 jobs.JobResult。
//
// 用於橋接 execution.Executor 與 jobs.JobExecutor 介面。
func ToJobResult(r *Result) *jobs.JobResult {
	if r == nil {
		return nil
	}
	return &jobs.JobResult{
		BackendName:    r.BackendName,
		BackendVersion: r.BackendVersion,
		Model:          r.Model,
		Attempt:        r.Attempt,
		StartedAt:      r.StartedAt,
		CompletedAt:    r.CompletedAt,
		Duration:       r.Duration,
		TraceID:        r.TraceID,
		OutputRef:      r.OutputRef,
		ErrorCode:      r.ErrorCode,
		ErrorMessage:   r.ErrorMessage,
	}
}

// NewJobExecutorAdapter 建立一個從 execution.Executor 到 jobs.JobExecutor 的轉接器。
//
// 讓 WorkerPool（依賴 jobs.JobExecutor）可以使用 execution.Executor 實作。
func NewJobExecutorAdapter(executor Executor) *JobExecutorAdapter {
	return &JobExecutorAdapter{executor: executor}
}

// JobExecutorAdapter 實作 jobs.JobExecutor 介面，委託給內部的 execution.Executor。
type JobExecutorAdapter struct {
	executor Executor
}

// Execute 實作 jobs.JobExecutor 介面。
func (a *JobExecutorAdapter) Execute(ctx context.Context, job *jobs.Job) (*jobs.JobResult, error) {
	result, err := a.executor.Execute(ctx, job)
	if err != nil {
		return nil, err
	}
	return ToJobResult(result), nil
}
