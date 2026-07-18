package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gorilla/mux"

	"github.com/user/audiocppruntime/internal/catalog"
	"github.com/user/audiocppruntime/internal/runtime"
)

type Config struct {
	Port        int
	Host        string
	BundleRoot  string
	CatalogPath string
	Backend     string
	Device      int
	Threads     int
	LoadTimeout int
}

type WebUIServer struct {
	cfg     Config
	manager *runtime.Manager
	cat     *catalog.Catalog
	router  *mux.Router
	tmpl    *template.Template
	mu      sync.RWMutex
}

func New(cfg Config) *WebUIServer {
	cat, err := catalog.LoadCatalog(cfg.CatalogPath)
	if err != nil {
		cat = defaultCatalog()
	}

	manager := runtime.NewManager(
		findServerExe(cfg.BundleRoot, cfg.Backend),
		cfg.BundleRoot,
		cfg.Host, cfg.Port, cfg.Device, cfg.Threads,
		cfg.Backend, cfg.LoadTimeout,
	)

	s := &WebUIServer{
		cfg:     cfg,
		manager: manager,
		cat:     cat,
		router:  mux.NewRouter(),
	}
	s.registerRoutes()
	s.registerUploadRoutes()
	return s
}

func (s *WebUIServer) Start() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	fmt.Printf("[runtime] WebUI starting at http://%s\n", addr)
	return http.ListenAndServe(addr, s.router)
}

func (s *WebUIServer) registerRoutes() {
	r := s.router

	r.HandleFunc("/", s.handleIndex).Methods("GET")
	r.HandleFunc("/health", s.handleHealth).Methods("GET")
	r.HandleFunc("/api/models", s.handleModels).Methods("GET")
	r.HandleFunc("/api/load", s.handleLoad).Methods("POST")
	r.HandleFunc("/api/unload", s.handleUnload).Methods("POST")
	r.HandleFunc("/api/status", s.handleStatus).Methods("GET")

	r.HandleFunc("/v1/models", s.proxyToServer).Methods("GET")
	r.HandleFunc("/v1/audio/speech", s.proxyToServer).Methods("POST")
	r.HandleFunc("/v1/audio/transcriptions", s.proxyToServer).Methods("POST")
	r.HandleFunc("/v1/tasks/run", s.proxyToServer).Methods("POST")

	staticDir := filepath.Join("web", "static")
	if _, err := os.Stat(staticDir); err == nil {
		r.PathPrefix("/static/").Handler(
			http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
	}
}

func (s *WebUIServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "zh"
	}

	entries := s.cat.Annotate(s.cfg.BundleRoot)
	var modelCards []ModelCard
	taskTabs := []TaskTab{
		{ID: "tts", Label: t(lang, "语音合成", "TTS"), Tasks: []string{"tts", "clon"}},
		{ID: "asr", Label: t(lang, "语音识别", "ASR"), Tasks: []string{"asr"}},
		{ID: "vc", Label: t(lang, "声音转换", "Voice Conversion"), Tasks: []string{"vc", "svc", "s2s"}},
		{ID: "gen", Label: t(lang, "音乐生成", "Music Gen"), Tasks: []string{"gen"}},
		{ID: "sep", Label: t(lang, "音源分离", "Source Sep"), Tasks: []string{"sep"}},
		{ID: "analyze", Label: t(lang, "音频分析", "Analysis"), Tasks: []string{"vad", "diar", "align"}},
		{ID: "vdes", Label: t(lang, "声音设计", "Voice Design"), Tasks: []string{"vdes"}},
	}

	for _, e := range entries {
		label := e.DisplayNameLabel(lang)
		if !e.Installed {
			label += t(lang, " · 未安装", " · not installed")
		}
		modelCards = append(modelCards, ModelCard{
			ID:       e.ID,
			Label:    label,
			Family:   e.Family,
			Task:     e.Task,
			Installed: e.Installed,
		})
	}

	data := IndexData{
		Title:     "audio.cpp Runtime",
		Models:    modelCards,
		Tabs:      taskTabs,
		Lang:      lang,
		ServerURL: s.manager.ServerURL(),
	}

	s.renderTemplate(w, "index", data)
}

func (s *WebUIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"alive":   s.manager.IsAlive(),
		"backend": s.cfg.Backend,
	})
}

func (s *WebUIServer) handleModels(w http.ResponseWriter, r *http.Request) {
	entries := s.cat.Annotate(s.cfg.BundleRoot)
	var result []map[string]interface{}
	for _, e := range entries {
		result = append(result, map[string]interface{}{
			"id":        e.ID,
			"label":     e.Label,
			"family":    e.Family,
			"task":      e.Task,
			"installed": e.Installed,
			"abs_path":  e.AbsPath,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *WebUIServer) handleLoad(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ModelID string `json:"model_id"`
		Mode    string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	entry := s.cat.EntryByID(req.ModelID)
	if entry == nil {
		http.Error(w, "model not found", http.StatusNotFound)
		return
	}
	absPath := filepath.Join(s.cfg.BundleRoot, entry.Path)
	absPath = filepath.Clean(absPath)
	mode := req.Mode
	if mode == "" {
		mode = entry.Mode
	}

	status, err := s.manager.EnsureLoaded(entry.ID, entry.Family, absPath, entry.Task, mode, entry.SessionOptions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": status})
}

func (s *WebUIServer) handleUnload(w http.ResponseWriter, r *http.Request) {
	s.manager.Unload()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "unloaded"})
}

func (s *WebUIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	alive := s.manager.IsAlive()
	modelID := ""
	if alive {
		modelID = s.manager.LoadedModelID()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"alive":    alive,
		"model_id": modelID,
		"backend":  s.cfg.Backend,
		"server":   s.manager.ServerURL(),
	})
}

func (s *WebUIServer) proxyToServer(w http.ResponseWriter, r *http.Request) {
	targetURL := s.manager.ServerURL() + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req, err := http.NewRequest(r.Method, targetURL, strings.NewReader(string(body)))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header = r.Header.Clone()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("proxy error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (s *WebUIServer) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	tmpl := template.Must(template.New(name).Funcs(template.FuncMap{
		"t": func(zh, en string) string {
			return t("zh", zh, en)
		},
	}).Parse(tplIndex))
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type IndexData struct {
	Title     string
	Models    []ModelCard
	Tabs      []TaskTab
	Lang      string
	ServerURL string
}

type ModelCard struct {
	ID        string
	Label     string
	Family    string
	Task      string
	Installed bool
}

type TaskTab struct {
	ID    string
	Label string
	Tasks []string
}

func t(lang, zh, en string) string {
	if lang == "en" {
		return en
	}
	return zh
}

func findServerExe(bundleRoot, backend string) string {
	candidates := []string{
		filepath.Join(bundleRoot, backend, "audiocpp_server.exe"),
		filepath.Join(bundleRoot, "gpu", "audiocpp_server.exe"),
		filepath.Join(bundleRoot, "cpu", "audiocpp_server.exe"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return filepath.Join(bundleRoot, backend, "audiocpp_server.exe")
}

func defaultCatalog() *catalog.Catalog {
	return &catalog.Catalog{
		Host: "127.0.0.1", Port: 8080, Device: 0, Threads: 1,
		Models: []catalog.ModelEntry{
			{ID: "qwen3-tts", DisplayName: "Qwen3-TTS 0.6B (tts)", Family: "qwen3_tts", Path: "models/Qwen3-TTS-12Hz-0.6B-Base", Task: "tts", Mode: "offline"},
			{ID: "vibevoice", DisplayName: "VibeVoice 1.5B (tts)", Family: "vibevoice", Path: "models/VibeVoice-1.5B", Task: "tts", Mode: "offline"},
			{ID: "qwen3-asr", DisplayName: "Qwen3-ASR 0.6B (asr)", Family: "qwen3_asr", Path: "models/Qwen3-ASR-0.6B", Task: "asr", Mode: "offline"},
		},
	}
}
