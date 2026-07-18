package catalog

type Catalog struct {
	Host    string       `json:"host"`
	Port    int          `json:"port"`
	Device  int          `json:"device"`
	Threads int          `json:"threads"`
	Models  []ModelEntry `json:"models"`
}

type ModelEntry struct {
	ID              string            `json:"id"`
	DisplayName     string            `json:"display_name,omitempty"`
	DisplayNameEn   string            `json:"display_name_en,omitempty"`
	Family          string            `json:"family"`
	Path            string            `json:"path"`
	Task            string            `json:"task"`
	Mode            string            `json:"mode"`
	DownloadID      string            `json:"download_id,omitempty"`
	MinVRAMGB       *float64          `json:"min_vram_gb,omitempty"`
	InputHint       string            `json:"input_hint,omitempty"`
	InputHintEn     string            `json:"input_hint_en,omitempty"`
	DefaultOptions  map[string]string `json:"default_options,omitempty"`
	LoadOptions     map[string]string `json:"load_options,omitempty"`
	SessionOptions  map[string]string `json:"session_options,omitempty"`
	ConfigID        string            `json:"config,omitempty"`
	WeightID        string            `json:"weight,omitempty"`
}
