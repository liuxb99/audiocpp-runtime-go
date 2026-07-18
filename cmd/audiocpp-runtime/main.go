package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/user/audiocppruntime/internal/server"
)

func main() {
	port := flag.Int("port", 7860, "WebUI port")
	host := flag.String("host", "127.0.0.1", "WebUI host")
	bundle := flag.String("bundle", "", "Bundle root directory (contains cpu/ gpu/ models/)")
	backend := flag.String("backend", "", "Backend: gpu|cuda|cpu (auto-detect if empty)")
	device := flag.Int("device", 0, "GPU device ID")
	threads := flag.Int("threads", 0, "Compute threads (0=auto)")
	loadTimeout := flag.Int("load-timeout", 300, "Model load timeout in seconds")
	flag.Parse()

	bundleRoot := *bundle
	if bundleRoot == "" {
		bundleRoot = findBundleRoot()
	}

	bk := *backend
	if bk == "" {
		bk = detectBackend(bundleRoot)
	}

	thr := *threads
	if thr <= 0 {
		if bk == "cpu" {
			thr = 4
		} else {
			thr = 1
		}
	}

	fmt.Printf("[runtime] Bundle root: %s\n", bundleRoot)
	fmt.Printf("[runtime] Backend: %s\n", bk)
	fmt.Printf("[runtime] WebUI: http://%s:%d\n", *host, *port)

	cfg := server.Config{
		Port:        *port,
		Host:        *host,
		BundleRoot:  bundleRoot,
		CatalogPath: filepath.Join("webui", "configs", "models_catalog.json"),
		Backend:     bk,
		Device:      *device,
		Threads:     thr,
		LoadTimeout: *loadTimeout,
	}

	srv := server.New(cfg)
	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func findBundleRoot() string {
	candidates := []string{
		".",
		"..",
		"../audiocpp-portable",
	}
	for _, c := range candidates {
		abs, _ := filepath.Abs(c)
		if hasBackendDir(abs) {
			return abs
		}
	}
	abs, _ := filepath.Abs(".")
	return abs
}

func hasBackendDir(root string) bool {
	for _, dir := range []string{"gpu", "cpu"} {
		info, err := os.Stat(filepath.Join(root, dir))
		if err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func detectBackend(bundleRoot string) string {
	nvcuda := filepath.Join(os.Getenv("SystemRoot"), "System32", "nvcuda.dll")
	if _, err := os.Stat(nvcuda); err == nil {
		if _, err := os.Stat(filepath.Join(bundleRoot, "gpu", "audiocpp_server.exe")); err == nil {
			return "cuda"
		}
	}
	if _, err := os.Stat(filepath.Join(bundleRoot, "cpu", "audiocpp_server.exe")); err == nil {
		return "cpu"
	}
	return "cuda"
}
