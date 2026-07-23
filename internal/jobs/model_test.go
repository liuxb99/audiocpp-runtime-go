package jobs

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/storage"
)

func TestJobToRecord(t *testing.T) {
	now := time.Now().UTC()
	started := now.Add(5 * time.Second)
	completed := now.Add(30 * time.Second)

	job := &Job{
		ID:      "job-1",
		Type:    TypeTTS,
		Status:  StatusSucceeded,
		ModelID: "model-1",
		Request: map[string]interface{}{
			"text":   "hello",
			"voice":  "default",
			"params": map[string]interface{}{"speed": 1.0},
		},
		Result: map[string]interface{}{
			"output_path": "/outputs/result.wav",
			"duration":    3.5,
		},
		Error:       "",
		Progress:    100.0,
		CreatedAt:   now,
		StartedAt:   &started,
		CompletedAt: &completed,
		Priority:    5,
	}

	rec := job.ToRecord()

	if rec.ID != "job-1" {
		t.Errorf("ID: got %q, want %q", rec.ID, "job-1")
	}
	if rec.Type != "tts" {
		t.Errorf("Type: got %q, want %q", rec.Type, "tts")
	}
	if rec.Status != "succeeded" {
		t.Errorf("Status: got %q, want %q", rec.Status, "succeeded")
	}
	if rec.ModelID != "model-1" {
		t.Errorf("ModelID: got %q, want %q", rec.ModelID, "model-1")
	}
	if rec.Progress != 100.0 {
		t.Errorf("Progress: got %f, want %f", rec.Progress, 100.0)
	}
	if rec.Priority != 5 {
		t.Errorf("Priority: got %d, want %d", rec.Priority, 5)
	}
	if !rec.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt mismatch")
	}
	if rec.StartedAt == nil || !rec.StartedAt.Equal(started) {
		t.Errorf("StartedAt mismatch")
	}
	if rec.CompletedAt == nil || !rec.CompletedAt.Equal(completed) {
		t.Errorf("CompletedAt mismatch")
	}

	var reqMap map[string]interface{}
	if err := json.Unmarshal([]byte(rec.Request), &reqMap); err != nil {
		t.Fatalf("Request should be valid JSON: %v", err)
	}
	if reqMap["text"] != "hello" {
		t.Errorf("Request.text: got %v, want hello", reqMap["text"])
	}

	if rec.Result == nil {
		t.Fatal("Result should not be nil")
	}
	var resMap map[string]interface{}
	if err := json.Unmarshal([]byte(*rec.Result), &resMap); err != nil {
		t.Fatalf("Result should be valid JSON: %v", err)
	}
	if resMap["output_path"] != "/outputs/result.wav" {
		t.Errorf("Result.output_path: got %v", resMap["output_path"])
	}

	if rec.Error != nil {
		t.Errorf("Error should be nil for empty error string, got %v", *rec.Error)
	}
}

func TestJobToRecord_NoResult(t *testing.T) {
	job := &Job{
		ID:      "job-2",
		Type:    TypeASR,
		Status:  StatusRunning,
		ModelID: "model-2",
		Request: map[string]interface{}{"audio": "test.wav"},
	}

	rec := job.ToRecord()
	if rec.Result != nil {
		t.Errorf("Result should be nil when job.Result is nil")
	}
}

func TestJobToRecord_WithError(t *testing.T) {
	job := &Job{
		ID:      "job-3",
		Type:    TypeTTS,
		Status:  StatusFailed,
		ModelID: "model-3",
		Request: map[string]interface{}{},
		Error:   "something went wrong",
	}

	rec := job.ToRecord()
	if rec.Error == nil {
		t.Fatal("Error should not be nil")
	}
	if *rec.Error != "something went wrong" {
		t.Errorf("Error: got %q, want %q", *rec.Error, "something went wrong")
	}
}

func TestJobFromRecord(t *testing.T) {
	now := time.Now().UTC()
	started := now.Add(5 * time.Second)
	completed := now.Add(30 * time.Second)
	errStr := "processing error"
	resultJSON := `{"output_path":"/outputs/result.wav","duration":3.5}`

	rec := &storage.JobRecord{
		ID:          "job-1",
		Type:        "tts",
		Status:      "failed",
		ModelID:     "model-1",
		Request:     `{"text":"hello","voice":"default"}`,
		Result:      &resultJSON,
		Error:       &errStr,
		Progress:    75.5,
		CreatedAt:   now,
		StartedAt:   &started,
		CompletedAt: &completed,
		Priority:    3,
	}

	job := JobFromRecord(rec)

	if job.ID != "job-1" {
		t.Errorf("ID: got %q, want %q", job.ID, "job-1")
	}
	if job.Type != Type("tts") {
		t.Errorf("Type: got %q, want %q", job.Type, Type("tts"))
	}
	if job.Status != Status("failed") {
		t.Errorf("Status: got %q, want %q", job.Status, Status("failed"))
	}
	if job.ModelID != "model-1" {
		t.Errorf("ModelID: got %q, want %q", job.ModelID, "model-1")
	}
	if job.Progress != 75.5 {
		t.Errorf("Progress: got %f, want %f", job.Progress, 75.5)
	}
	if job.Priority != 3 {
		t.Errorf("Priority: got %d, want %d", job.Priority, 3)
	}
	if !job.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt mismatch")
	}
	if job.StartedAt == nil || !job.StartedAt.Equal(started) {
		t.Errorf("StartedAt mismatch")
	}
	if job.CompletedAt == nil || !job.CompletedAt.Equal(completed) {
		t.Errorf("CompletedAt mismatch")
	}

	if job.Request["text"] != "hello" {
		t.Errorf("Request.text: got %v", job.Request["text"])
	}
	if job.Request["voice"] != "default" {
		t.Errorf("Request.voice: got %v", job.Request["voice"])
	}

	if job.Result["output_path"] != "/outputs/result.wav" {
		t.Errorf("Result.output_path: got %v", job.Result["output_path"])
	}
	if job.Result["duration"] != 3.5 {
		t.Errorf("Result.duration: got %v", job.Result["duration"])
	}

	if job.Error != "processing error" {
		t.Errorf("Error: got %q, want %q", job.Error, "processing error")
	}
}

func TestJobFromRecord_NilOptionalFields(t *testing.T) {
	now := time.Now().UTC()
	rec := &storage.JobRecord{
		ID:        "job-2",
		Type:      "asr",
		Status:    "pending",
		ModelID:   "model-2",
		Request:   `{}`,
		Result:    nil,
		Error:     nil,
		Progress:  0,
		CreatedAt: now,
		Priority:  1,
	}

	job := JobFromRecord(rec)

	if job.Result != nil {
		t.Errorf("Result should be nil, got %v", job.Result)
	}
	if job.Error != "" {
		t.Errorf("Error should be empty, got %q", job.Error)
	}
}

func TestJobFromRecord_InvalidRequestJSON(t *testing.T) {
	rec := &storage.JobRecord{
		ID:      "job-3",
		Type:    "tts",
		Status:  "pending",
		Request: `not-json`,
	}

	job := JobFromRecord(rec)
	if job.Request == nil {
		t.Error("Request should be non-nil empty map on invalid JSON")
	}
	if len(job.Request) != 0 {
		t.Errorf("expected empty request map, got %v", job.Request)
	}
}

func TestJobFromRecord_InvalidResultJSON(t *testing.T) {
	badResult := `not-json`
	rec := &storage.JobRecord{
		ID:      "job-4",
		Type:    "tts",
		Status:  "completed",
		Request: `{}`,
		Result:  &badResult,
	}

	job := JobFromRecord(rec)
	if job.Result != nil {
		t.Error("Result should be nil when JSON is invalid")
	}
}

func TestTypeIsValid_ValidTypes(t *testing.T) {
	valid := []Type{
		TypeTTS, TypeASR, TypeVoiceClone, TypeVoiceConversion,
		TypeSourceSep, TypeMusicGen, TypeVAD, TypeDiarization,
		TypeAlignment, TypeVoiceDesign,
	}
	for _, typ := range valid {
		if !typ.IsValid() {
			t.Errorf("expected %q to be valid", typ)
		}
	}
}

func TestTypeIsValid_InvalidType(t *testing.T) {
	if Type("unknown").IsValid() {
		t.Error("expected 'unknown' type to be invalid")
	}
	if Type("").IsValid() {
		t.Error("expected empty type to be invalid")
	}
	typ := Type("custom_type")
	if typ.IsValid() {
		t.Errorf("expected %q to be invalid", typ)
	}
}

func TestStatusIsValid_ValidStatuses(t *testing.T) {
	valid := []Status{
		StatusPending, StatusQueued, StatusRunning,
		StatusSucceeded, StatusFailed, StatusCanceled,
		StatusCancelRequested, StatusRetryWaiting, StatusTimedOut,
	}
	for _, s := range valid {
		if !s.IsValid() {
			t.Errorf("expected %q to be valid", s)
		}
	}
}

func TestStatusIsValid_InvalidStatus(t *testing.T) {
	if Status("unknown").IsValid() {
		t.Error("expected 'unknown' status to be invalid")
	}
	if Status("").IsValid() {
		t.Error("expected empty status to be invalid")
	}
}

func TestStatusIsTerminal_TerminalStatuses(t *testing.T) {
	terminal := []Status{StatusSucceeded, StatusCanceled, StatusTimedOut}
	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("expected %q to be terminal", s)
		}
	}
}

func TestStatusIsTerminal_NonTerminalStatuses(t *testing.T) {
	nonTerminal := []Status{StatusPending, StatusQueued, StatusRunning, StatusCancelRequested, StatusRetryWaiting}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("expected %q to be non-terminal", s)
		}
	}
}
