package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func (s *WebUIServer) registerUploadRoutes() {
	s.router.HandleFunc("/api/upload", s.handleUpload).Methods("POST")
	s.router.HandleFunc("/api/upload-multi", s.handleUploadMulti).Methods("POST")
}

func (s *WebUIServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(100 << 20)
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := filepath.Ext(header.Filename)
	tmpFile, err := os.CreateTemp("", "audiocpp_up_*"+ext)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, file); err != nil {
		os.Remove(tmpFile.Name())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"path": tmpFile.Name()})
}

func (s *WebUIServer) handleUploadMulti(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(100 << 20)
	paths := make(map[string]string)

	for _, name := range []string{"source", "target", "file"} {
		file, header, err := r.FormFile(name)
		if err != nil {
			continue
		}
		defer file.Close()

		ext := filepath.Ext(header.Filename)
		tmpFile, err := os.CreateTemp("", "audiocpp_"+name+"_*"+ext)
		if err != nil {
			continue
		}
		defer tmpFile.Close()

		io.Copy(tmpFile, file)
		paths[name] = tmpFile.Name()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(paths)
}
