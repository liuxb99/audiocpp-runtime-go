package outputs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/liuxb99/audiocpp-runtime-go/internal/storage"
)

var mimeExtMap = map[string]string{
	"audio/wav":                ".wav",
	"audio/x-wav":              ".wav",
	"audio/vnd.wave":           ".wav",
	"audio/mpeg":               ".mp3",
	"audio/mp3":                ".mp3",
	"audio/mp4":                ".m4a",
	"audio/x-m4a":              ".m4a",
	"audio/ogg":                ".ogg",
	"audio/vorbis":             ".ogg",
	"audio/flac":               ".flac",
	"audio/x-flac":             ".flac",
	"audio/webm":               ".webm",
	"audio/aac":                ".aac",
	"audio/x-aac":              ".aac",
	"audio/wma":                ".wma",
	"audio/x-ms-wma":           ".wma",
	"audio/opus":               ".opus",
	"audio/amr":                ".amr",
	"audio/L16":                ".raw",
	"audio/L8":                 ".raw",
	"application/octet-stream": ".bin",
}

func extForMIME(mimeType string) string {
	if mimeType == "" {
		return ".bin"
	}
	mt := strings.ToLower(strings.TrimSpace(mimeType))
	if ext, ok := mimeExtMap[mt]; ok {
		return ext
	}
	base := mt
	if idx := strings.IndexByte(mt, ';'); idx != -1 {
		base = strings.TrimSpace(mt[:idx])
		if ext, ok := mimeExtMap[base]; ok {
			return ext
		}
	}
	if idx := strings.IndexByte(base, '/'); idx != -1 && idx+1 < len(base) {
		sub := base[idx+1:]
		if !strings.ContainsAny(sub, "+.") {
			return "." + sub
		}
	}
	return ".bin"
}

type Manager struct {
	rootDir    string
	retainDays int
	repo       *storage.OutputsRepository
}

func NewManager(rootDir string, retainDays int, repo *storage.OutputsRepository) *Manager {
	return &Manager{
		rootDir:    rootDir,
		retainDays: retainDays,
		repo:       repo,
	}
}

func (m *Manager) Get(id string) (*storage.OutputRecord, error) {
	return m.repo.Get(id)
}

func (m *Manager) Save(ctx context.Context, jobID, outputType, mimeType string, data []byte, metadata map[string]interface{}) (*storage.OutputRecord, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("outputs: cannot save empty data for job %s", jobID)
	}

	id := uuid.New().String()
	ext := extForMIME(mimeType)
	relPath := filepath.Join(jobID, id+ext)
	absPath := filepath.Join(m.rootDir, relPath)

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return nil, fmt.Errorf("outputs: mkdir %s: %w", filepath.Dir(absPath), err)
	}

	if err := os.WriteFile(absPath, data, 0644); err != nil {
		return nil, fmt.Errorf("outputs: write %s: %w", absPath, err)
	}

	var metaStr string
	if metadata != nil {
		b, err := json.Marshal(metadata)
		if err != nil {
			os.Remove(absPath)
			return nil, fmt.Errorf("outputs: marshal metadata: %w", err)
		}
		metaStr = string(b)
	}

	record := &storage.OutputRecord{
		ID:        id,
		JobID:     jobID,
		Type:      outputType,
		Path:      absPath,
		MimeType:  mimeType,
		SizeBytes: int64(len(data)),
		Metadata:  metaStr,
		CreatedAt: time.Now().UTC(),
	}

	if err := m.repo.Create(record); err != nil {
		os.Remove(absPath)
		return nil, fmt.Errorf("outputs: db create: %w", err)
	}

	return record, nil
}

func (m *Manager) GetPath(record *storage.OutputRecord) string {
	return record.Path
}

func (m *Manager) Delete(ctx context.Context, id string) error {
	record, err := m.repo.Get(id)
	if err != nil {
		return fmt.Errorf("outputs: get %s: %w", id, err)
	}

	if record.Path != "" {
		if err := os.Remove(record.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("outputs: remove file %s: %w", record.Path, err)
		}
		_ = removeEmptyParentDirs(filepath.Dir(record.Path), m.rootDir)
	}

	if err := m.repo.Delete(id); err != nil {
		return fmt.Errorf("outputs: db delete %s: %w", id, err)
	}

	return nil
}

func (m *Manager) Cleanup(ctx context.Context) (int64, error) {
	if m.retainDays <= 0 {
		return 0, nil
	}
	before := time.Now().UTC().AddDate(0, 0, -m.retainDays)
	records, err := m.repo.ListOlderThan(before)
	if err != nil {
		return 0, fmt.Errorf("outputs: list older than %s: %w", before.Format(time.RFC3339), err)
	}

	var count int64
	for _, rec := range records {
		if rec.Path != "" {
			if err := os.Remove(rec.Path); err != nil && !os.IsNotExist(err) {
				return count, fmt.Errorf("outputs: cleanup remove file %s: %w", rec.Path, err)
			}
			_ = removeEmptyParentDirs(filepath.Dir(rec.Path), m.rootDir)
		}
		if err := m.repo.Delete(rec.ID); err != nil {
			return count, fmt.Errorf("outputs: cleanup delete db %s: %w", rec.ID, err)
		}
		count++
	}
	return count, nil
}

func (m *Manager) ListByJob(ctx context.Context, jobID string) ([]*storage.OutputRecord, error) {
	records, err := m.repo.ListByJob(jobID)
	if err != nil {
		return nil, fmt.Errorf("outputs: list by job %s: %w", jobID, err)
	}
	return records, nil
}

func removeEmptyParentDirs(dir, root string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	for strings.HasPrefix(absDir, absRoot) && absDir != absRoot {
		empty, err := isDirEmpty(absDir)
		if err != nil {
			return err
		}
		if !empty {
			break
		}
		if err := os.Remove(absDir); err != nil {
			return err
		}
		absDir = filepath.Dir(absDir)
	}
	return nil
}

func isDirEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil
}
