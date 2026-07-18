package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type Manager struct {
	mu sync.Mutex

	serverExe   string
	bundleRoot  string
	host        string
	port        int
	backend     string
	device      int
	threads     int
	loadTimeout time.Duration

	cmd     *exec.Cmd
	modelID string
	logPath string
}

func NewManager(serverExe, bundleRoot, host string, port, device, threads int, backend string, loadTimeoutSec int) *Manager {
	return &Manager{
		serverExe:   serverExe,
		bundleRoot:  bundleRoot,
		host:        host,
		port:        port,
		backend:     backend,
		device:      device,
		threads:     threads,
		loadTimeout: time.Duration(loadTimeoutSec) * time.Second,
	}
}

func (m *Manager) EnsureLoaded(modelID, family, path, task, mode string, sessionOptions map[string]string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := &ServerConfig{
		Host:    m.host,
		Port:    m.port,
		Backend: m.backend,
		Device:  m.device,
		Threads: m.threads,
		Models: []ServerModelConfig{{
			ID:             modelID,
			Family:         family,
			Path:           path,
			Task:           task,
			Mode:           mode,
			SessionOptions: sessionOptions,
		}},
	}

	cfgPath, err := WriteTempConfig(cfg)
	if err != nil {
		return "", fmt.Errorf("write config: %w", err)
	}
	defer os.Remove(cfgPath)

	if err := m.start(cfgPath); err != nil {
		return "", err
	}

	return fmt.Sprintf("Loaded: %s", modelID), nil
}

func (m *Manager) start(cfgPath string) error {
	m.stop()
	cmd := exec.Command(m.serverExe,
		"--config", cfgPath,
		"--host", m.host,
		"--port", fmt.Sprint(m.port),
	)
	cmd.Dir = m.bundleRoot
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start server: %w", err)
	}
	m.cmd = cmd
	go m.wait()
	alive, err := m.waitForHealth()
	if err != nil {
		m.stop()
		return fmt.Errorf("server health check: %w (log: %s)", err, stdout.String()[:min(len(stdout.String()), 2000)])
	}
	if !alive {
		m.stop()
		return fmt.Errorf("server failed to become healthy (log: %s)", stdout.String()[:min(len(stdout.String()), 2000)])
	}
	return nil
}

func (m *Manager) wait() {
	m.cmd.Wait()
}

func (m *Manager) waitForHealth() (bool, error) {
	deadline := time.Now().Add(m.loadTimeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("http://%s:%d/health", m.host, m.port))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return true, nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false, nil
}

func (m *Manager) stop() {
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
		m.cmd.Wait()
		m.cmd = nil
	}
}

func (m *Manager) Unload() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stop()
}

func (m *Manager) IsAlive() bool {
	resp, err := http.Get(fmt.Sprintf("http://%s:%d/health", m.host, m.port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func (m *Manager) ServerURL() string {
	return fmt.Sprintf("http://%s:%d", m.host, m.port)
}

func (m *Manager) FindGGUFExe() string {
	candidates := []string{
		filepath.Join(m.bundleRoot, m.backend, "audiocpp_gguf.exe"),
		filepath.Join(m.bundleRoot, "gpu", "audiocpp_gguf.exe"),
		filepath.Join(m.bundleRoot, "cpu", "audiocpp_gguf.exe"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func (m *Manager) LoadedModelID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	resp, err := http.Get(fmt.Sprintf("http://%s:%d/v1/models", m.host, m.port))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}
	if len(result.Data) > 0 {
		return result.Data[0].ID
	}
	return ""
}

func proxyRequest(targetURL string, w http.ResponseWriter, r *http.Request) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header = r.Header.Clone()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	return err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
