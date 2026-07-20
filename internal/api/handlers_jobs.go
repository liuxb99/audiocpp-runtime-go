package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/liuxb99/audiocpp-runtime-go/internal/jobs"
)

type createJobRequest struct {
	Type     string                 `json:"type"`
	ModelID  string                 `json:"model_id"`
	Request  map[string]interface{} `json:"request"`
	Priority int                    `json:"priority,omitempty"`
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req createJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body")
		return
	}

	if req.Type == "" || req.ModelID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "type and model_id are required")
		return
	}

	if _, exists := s.modelReg.Get(req.ModelID); !exists {
		writeError(w, http.StatusNotFound, "MODEL_NOT_FOUND", "Model not found: "+req.ModelID)
		return
	}

	job := &jobs.Job{
		ID:       uuid.New().String(),
		Type:     jobs.Type(req.Type),
		Status:   jobs.StatusPending,
		ModelID:  req.ModelID,
		Request:  req.Request,
		Priority: req.Priority,
	}

	if !job.Type.IsValid() {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid job type: "+req.Type)
		return
	}

	if err := s.jobManager.CreateJob(job); err != nil {
		writeError(w, http.StatusInternalServerError, "JOB_CREATE_ERROR", err.Error())
		return
	}

	if err := s.jobManager.Enqueue(job); err != nil {
		writeError(w, http.StatusInternalServerError, "JOB_ENQUEUE_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, job)
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	offset := 0
	status := r.URL.Query().Get("status")

	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	jobs, err := s.jobManager.ListJobs(limit, offset, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LIST_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, jobs)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	job, err := s.jobManager.GetJob(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("Job %s not found", id))
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	job, err := s.jobManager.GetJob(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("Job %s not found", id))
		return
	}

	if job.Status.IsTerminal() {
		writeError(w, http.StatusBadRequest, "INVALID_STATE", fmt.Sprintf("Job %s is already in terminal state: %s", id, job.Status))
		return
	}

	if err := s.jobManager.CancelJob(job); err != nil {
		writeError(w, http.StatusInternalServerError, "CANCEL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleJobOutputs(w http.ResponseWriter, r *http.Request) {
	jobID := mux.Vars(r)["id"]

	_, err := s.jobManager.GetJob(jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("Job %s not found", jobID))
		return
	}

	outputs, err := s.outputMgr.ListByJob(r.Context(), jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LIST_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, outputs)
}

func (s *Server) handleGetOutput(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	output, err := s.outputMgr.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("Output %s not found", id))
		return
	}

	writeJSON(w, http.StatusOK, output)
}
