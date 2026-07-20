package audiocpp

import "strings"

type Capability string

const (
	CapTTS              Capability = "tts"
	CapASR              Capability = "asr"
	CapVoiceClone       Capability = "voice_clone"
	CapVoiceConversion  Capability = "voice_conversion"
	CapSourceSeparation Capability = "source_separation"
	CapMusicGeneration  Capability = "music_generation"
	CapVAD              Capability = "vad"
	CapDiarization      Capability = "diarization"
	CapAlignment        Capability = "alignment"
	CapVoiceDesign      Capability = "voice_design"
)

var taskToCaps = map[string][]Capability{
	"tts":               {CapTTS},
	"asr":               {CapASR},
	"voice_clone":       {CapVoiceClone, CapTTS},
	"voice_conversion":  {CapVoiceConversion},
	"source_separation": {CapSourceSeparation},
	"music_generation":  {CapMusicGeneration},
	"vad":               {CapVAD},
	"diarization":       {CapDiarization},
	"alignment":         {CapAlignment},
	"voice_design":      {CapVoiceDesign},
}

var capToTask = map[Capability]string{
	CapTTS:              "tts",
	CapASR:              "asr",
	CapVoiceClone:       "voice_clone",
	CapVoiceConversion:  "voice_conversion",
	CapSourceSeparation: "source_separation",
	CapMusicGeneration:  "music_generation",
	CapVAD:              "vad",
	CapDiarization:      "diarization",
	CapAlignment:        "alignment",
	CapVoiceDesign:      "voice_design",
}

func TaskToCapabilities(task string) []Capability {
	task = strings.ToLower(task)
	if caps, ok := taskToCaps[task]; ok {
		result := make([]Capability, len(caps))
		copy(result, caps)
		return result
	}
	return nil
}

func TaskFromCapability(c Capability) string {
	if task, ok := capToTask[c]; ok {
		return task
	}
	return string(c)
}
