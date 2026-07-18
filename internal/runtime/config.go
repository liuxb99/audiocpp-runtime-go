package runtime

import (
	"encoding/json"
	"os"
)

type ServerModelConfig struct {
	ID                    string            `json:"id"`
	Family                string            `json:"family"`
	Path                  string            `json:"path"`
	Task                  string            `json:"task"`
	Mode                  string            `json:"mode"`
	ConfigID              string            `json:"config,omitempty"`
	WeightID              string            `json:"weight,omitempty"`
	LoadOptions           map[string]string `json:"load_options,omitempty"`
	SessionOptions        map[string]string `json:"session_options,omitempty"`
	DefaultVoicePresetID  string            `json:"default_voice_preset_id,omitempty"`
}

type ServerConfig struct {
	Host         string              `json:"host"`
	Port         int                 `json:"port"`
	Backend      string              `json:"backend"`
	Device       int                 `json:"device"`
	Threads      int                 `json:"threads"`
	LazyLoad     bool                `json:"lazy_load,omitempty"`
	Models       []ServerModelConfig `json:"models"`
}

func WriteTempConfig(cfg *ServerConfig) (string, error) {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "audiocpp_cfg_*.json")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
