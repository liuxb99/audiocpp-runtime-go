package jobs

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/audiocpp"
)

type WorkerPool struct {
	manager *Manager
	client  *audiocpp.Client
	queue   *Queue
	workers int
	stopCh  chan struct{}
	wg      sync.WaitGroup
	running int32
}

func NewWorkerPool(manager *Manager, client *audiocpp.Client, workers int) *WorkerPool {
	return &WorkerPool{
		manager: manager,
		client:  client,
		queue:   manager.queue,
		workers: workers,
		stopCh:  make(chan struct{}),
	}
}

func (wp *WorkerPool) Start() {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.run(i)
	}
	log.Printf("[jobs] started %d workers", wp.workers)
}

func (wp *WorkerPool) Stop() {
	close(wp.stopCh)
	wp.wg.Wait()
	log.Printf("[jobs] all workers stopped")
}

func (wp *WorkerPool) run(id int) {
	defer wp.wg.Done()
	for {
		select {
		case <-wp.stopCh:
			return
		default:
			job := wp.queue.Dequeue()
			if job == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			wp.process(job)
		}
	}
}

func (wp *WorkerPool) process(job *Job) {
	log.Printf("[jobs] worker picked up job %s (type=%s)", job.ID, job.Type)

	if err := wp.manager.StartJob(job); err != nil {
		log.Printf("[jobs] failed to start job %s: %v", job.ID, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	result, err := wp.execute(ctx, job)
	if err != nil {
		log.Printf("[jobs] job %s failed: %v", job.ID, err)
		if updateErr := wp.manager.FailJob(job, err); updateErr != nil {
			log.Printf("[jobs] failed to persist failure for job %s: %v", job.ID, updateErr)
		}
		return
	}

	if err := wp.manager.CompleteJob(job, result); err != nil {
		log.Printf("[jobs] failed to persist completion for job %s: %v", job.ID, err)
	}
}

func (wp *WorkerPool) execute(ctx context.Context, job *Job) (map[string]interface{}, error) {
	switch job.Type {
	case TypeTTS:
		return wp.executeTTS(ctx, job)
	case TypeASR:
		return wp.executeASR(ctx, job)
	default:
		return wp.executeTask(ctx, job)
	}
}

func (wp *WorkerPool) executeTTS(ctx context.Context, job *Job) (map[string]interface{}, error) {
	req := &audiocpp.SpeechRequest{
		Model: job.ModelID,
	}

	if v, ok := job.Request["input"].(string); ok {
		req.Input = v
	} else {
		return nil, fmt.Errorf("tts request missing 'input' field")
	}
	if v, ok := job.Request["voice"].(string); ok {
		req.Voice = v
	}
	if v, ok := job.Request["language"].(string); ok {
		req.Language = v
	}
	if v, ok := job.Request["response_format"].(string); ok {
		req.ResponseFormat = v
	}
	if v, ok := getFloat64(job.Request["temperature"]); ok {
		req.Temperature = v
	}
	if v, ok := getInt(job.Request["top_k"]); ok {
		req.TopK = v
	}
	if v, ok := getFloat64(job.Request["top_p"]); ok {
		req.TopP = v
	}
	if v, ok := getInt(job.Request["seed"]); ok {
		req.Seed = v
	}

	resp, err := wp.client.Speech(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("tts request: %w", err)
	}
	defer resp.Body.Close()

	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read tts response: %w", err)
	}

	return map[string]interface{}{
		"audio_data": audioBytes,
		"format":     resp.Header.Get("Content-Type"),
	}, nil
}

func (wp *WorkerPool) executeASR(ctx context.Context, job *Job) (map[string]interface{}, error) {
	req := &audiocpp.TranscribeRequest{
		Model: job.ModelID,
	}

	if v, ok := job.Request["audio"].(string); ok {
		req.Audio = v
	} else {
		return nil, fmt.Errorf("asr request missing 'audio' field")
	}
	if v, ok := job.Request["language"].(string); ok {
		req.Language = v
	}

	resp, err := wp.client.TranscribeJSON(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("asr request: %w", err)
	}

	result := map[string]interface{}{
		"text": resp.Text,
	}
	if resp.Timing != nil {
		result["timing"] = resp.Timing
	}
	return result, nil
}

func (wp *WorkerPool) executeTask(ctx context.Context, job *Job) (map[string]interface{}, error) {
	req := &audiocpp.TaskRequest{
		Model:   job.ModelID,
		Request: job.Request,
	}

	resp, err := wp.client.RunTask(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("task execution: %w", err)
	}

	result := make(map[string]interface{})
	if resp.Text != "" {
		result["text"] = resp.Text
	}
	if resp.Audio != "" {
		result["audio"] = resp.Audio
	}
	if resp.SampleRate > 0 {
		result["sample_rate"] = resp.SampleRate
	}
	if resp.Channels > 0 {
		result["channels"] = resp.Channels
	}
	if len(resp.NamedAudioOutputs) > 0 {
		result["named_audio_outputs"] = resp.NamedAudioOutputs
	}
	if len(resp.Segments) > 0 {
		result["segments"] = resp.Segments
	}
	if len(resp.SpeakerTurns) > 0 {
		result["speaker_turns"] = resp.SpeakerTurns
	}
	if len(resp.Words) > 0 {
		result["words"] = resp.Words
	}
	if resp.Timing != nil {
		result["timing"] = resp.Timing
	}
	return result, nil
}

func getFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	default:
		return 0, false
	}
}

func getInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case float64:
		return int(val), true
	case int64:
		return int(val), true
	default:
		return 0, false
	}
}
