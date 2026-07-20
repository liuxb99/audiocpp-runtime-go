package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func main() {
	serverAddr := flag.String("server", "http://127.0.0.1:8091", "server address")
	timeout := flag.Int("timeout", 30, "request timeout in seconds")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	cmd := args[0]
	cmdArgs := args[1:]

	client := &http.Client{Timeout: time.Duration(*timeout) * time.Second}
	baseURL := *serverAddr

	switch cmd {
	case "health":
		handleHealth(client, baseURL)
	case "models":
		handleModels(client, baseURL, cmdArgs)
	case "tts":
		handleTTS(client, baseURL, cmdArgs)
	case "asr":
		handleASR(client, baseURL, cmdArgs)
	case "jobs":
		handleJobs(client, baseURL, cmdArgs)
	case "voices":
		handleVoices(client, baseURL, cmdArgs)
	case "capabilities":
		handleCapabilities(client, baseURL)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: audiocppctl [options] <command> [args]

Commands:
  health                    Check server health
  models [id]              List models or get model details
  tts --model <id> --text <text> [options]  Generate speech
  asr --model <id> --audio <file>  Transcribe audio
  jobs [id]                List jobs or get job details
  voices [model]           List available voices
  capabilities             List capabilities

Options:
  --server <url>           Server address (default: http://127.0.0.1:8091)
  --timeout <seconds>      Request timeout (default: 30)
`)
}

func handleHealth(client *http.Client, baseURL string) {
	resp, err := client.Get(baseURL + "/v1/health")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
}

func handleModels(client *http.Client, baseURL string, args []string) {
	var url string
	if len(args) > 0 {
		url = baseURL + "/v1/models/" + args[0]
	} else {
		url = baseURL + "/v1/models"
	}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
}

func handleTTS(client *http.Client, baseURL string, args []string) {
	model := ""
	text := ""
	voice := ""
	lang := ""
	out := "output.wav"

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--model":
			if i+1 < len(args) {
				model = args[i+1]
				i++
			}
		case "--text":
			if i+1 < len(args) {
				text = args[i+1]
				i++
			}
		case "--voice":
			if i+1 < len(args) {
				voice = args[i+1]
				i++
			}
		case "--lang":
			if i+1 < len(args) {
				lang = args[i+1]
				i++
			}
		case "--out":
			if i+1 < len(args) {
				out = args[i+1]
				i++
			}
		}
	}

	if model == "" || text == "" {
		fmt.Fprintln(os.Stderr, "Usage: audiocppctl tts --model <id> --text <text> [--voice <v>] [--lang <l>] [--out <file>]")
		os.Exit(1)
	}

	body := map[string]interface{}{
		"model": model,
		"input": text,
	}
	if voice != "" {
		body["voice"] = voice
	}
	if lang != "" {
		body["language"] = lang
	}

	data, _ := json.Marshal(body)
	resp, err := client.Post(baseURL+"/v1/tts", "application/json", bytes.NewReader(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Error: %s\n", string(body))
		os.Exit(1)
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(out, audioData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Audio saved to %s (%d bytes)\n", out, len(audioData))
}

func handleASR(client *http.Client, baseURL string, args []string) {
	model := ""
	audioFile := ""
	lang := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--model":
			if i+1 < len(args) {
				model = args[i+1]
				i++
			}
		case "--audio":
			if i+1 < len(args) {
				audioFile = args[i+1]
				i++
			}
		case "--lang":
			if i+1 < len(args) {
				lang = args[i+1]
				i++
			}
		}
	}

	if model == "" || audioFile == "" {
		fmt.Fprintln(os.Stderr, "Usage: audiocppctl asr --model <id> --audio <file> [--lang <lang>]")
		os.Exit(1)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writer.WriteField("model", model)
	if lang != "" {
		writer.WriteField("language", lang)
	}
	file, err := os.Open(audioFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()
	part, err := writer.CreateFormFile("audio", filepath.Base(audioFile))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating form: %v\n", err)
		os.Exit(1)
	}
	io.Copy(part, file)
	writer.Close()

	resp, err := client.Post(baseURL+"/v1/asr", writer.FormDataContentType(), &body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	fmt.Println(string(respBody))
}

func handleJobs(client *http.Client, baseURL string, args []string) {
	var url string
	if len(args) > 0 {
		url = baseURL + "/v1/jobs/" + args[0]
	} else {
		url = baseURL + "/v1/jobs"
	}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
}

func handleVoices(client *http.Client, baseURL string, args []string) {
	url := baseURL + "/v1/voices"
	if len(args) > 0 {
		url += "?model=" + args[0]
	}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
}

func handleCapabilities(client *http.Client, baseURL string) {
	resp, err := client.Get(baseURL + "/v1/capabilities")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
}
