package api

import (
	"net/http"

	"github.com/gorilla/mux"
)

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	models := s.modelReg.List()
	writeJSON(w, http.StatusOK, models)
}

func (s *Server) handleGetModel(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	model, exists := s.modelReg.Get(id)
	if !exists {
		writeError(w, http.StatusNotFound, "MODEL_NOT_FOUND", "Model not found: "+id)
		return
	}
	writeJSON(w, http.StatusOK, model)
}
