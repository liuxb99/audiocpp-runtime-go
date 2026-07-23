package jobs

import (
	"context"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/execution"
)

// ResultToMap 將 execution.Result 轉換為 map[string]interface{}，
// 用於儲存在 Job.Result 欄位中。
func ResultToMap(r *execution.Result) map[string]interface{} {
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
func ApplyResultToJob(job *Job, r *execution.Result) {
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

// ToJobResult 將 execution.Result 轉換為 JobResult。
//
// 用於橋接 execution.Executor 與 JobExecutor 介面。
func ToJobResult(r *execution.Result) *JobResult {
	if r == nil {
		return nil
	}
	return &JobResult{
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

// NewJobExecutorAdapter 建立一個從 execution.Executor 到 JobExecutor 的轉接器。
//
// 讓 WorkerPool（依賴 JobExecutor）可以使用 execution.Executor 實作。
func NewJobExecutorAdapter(executor execution.Executor) *JobExecutorAdapter {
	return &JobExecutorAdapter{executor: executor}
}

// JobExecutorAdapter 實作 JobExecutor 介面，委託給內部的 execution.Executor。
type JobExecutorAdapter struct {
	executor execution.Executor
}

// Execute 實作 JobExecutor 介面。
func (a *JobExecutorAdapter) Execute(ctx context.Context, job *Job) (*JobResult, error) {
	// 將 Job 轉換為 ExecutionRequest
	req := &execution.ExecutionRequest{
		JobID:   job.ID,
		Type:    string(job.Type),
		ModelID: job.ModelID,
		Request: job.Request,
		Attempt: job.Attempt,
		TraceID: job.TraceID,
	}
	result, err := a.executor.Execute(ctx, req)
	if err != nil {
		return nil, err
	}
	return ToJobResult(result), nil
}
