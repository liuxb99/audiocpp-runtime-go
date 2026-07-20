package audiocpp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	timeout    time.Duration
}

func NewClient(host string, port int, timeout time.Duration) *Client {
	return &Client{
		baseURL:    fmt.Sprintf("http://%s:%d", host, port),
		httpClient: &http.Client{Timeout: timeout},
		timeout:    timeout,
	}
}

func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	var result HealthResponse
	err := c.doJSONRequest(ctx, http.MethodGet, "/health", nil, &result)
	return &result, err
}

func (c *Client) ListModels(ctx context.Context) (*ModelsResponse, error) {
	var result ModelsResponse
	err := c.doJSONRequest(ctx, http.MethodGet, "/v1/models", nil, &result)
	return &result, err
}

func (c *Client) Speech(ctx context.Context, req *SpeechRequest) (*http.Response, error) {
	return c.doRequest(ctx, http.MethodPost, "/v1/audio/speech", req, "application/json")
}

func (c *Client) TranscribeJSON(ctx context.Context, req *TranscribeRequest) (*TranscribeResponse, error) {
	var result TranscribeResponse
	err := c.doJSONRequest(ctx, http.MethodPost, "/v1/audio/transcriptions", req, &result)
	return &result, err
}

func (c *Client) TranscribeMultipart(ctx context.Context, modelID string, audioPath string, opts map[string]string) (*TranscribeResponse, error) {
	audioFile, err := os.Open(audioPath)
	if err != nil {
		return nil, NewError(ErrInvalidRequest, "failed to open audio file", err.Error())
	}
	defer audioFile.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	if err := mw.WriteField("model", modelID); err != nil {
		return nil, NewError(ErrInternal, "failed to write model field", err.Error())
	}

	aw, err := mw.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return nil, NewError(ErrInternal, "failed to create form file", err.Error())
	}
	if _, err := io.Copy(aw, audioFile); err != nil {
		return nil, NewError(ErrInternal, "failed to copy audio data", err.Error())
	}

	for k, v := range opts {
		if err := mw.WriteField(k, v); err != nil {
			return nil, NewError(ErrInternal, "failed to write option field", err.Error())
		}
	}
	mw.Close()

	resp, err := c.doRequest(ctx, http.MethodPost, "/v1/audio/transcriptions", &buf, mw.FormDataContentType())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewError(ErrInternal, "failed to read response body", err.Error())
	}

	var result TranscribeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, NewError(ErrInternal, "failed to parse response", err.Error())
	}
	return &result, nil
}

func (c *Client) RunTask(ctx context.Context, req *TaskRequest) (*TaskResponse, error) {
	var result TaskResponse
	err := c.doJSONRequest(ctx, http.MethodPost, "/v1/task", req, &result)
	return &result, err
}

func (c *Client) ListVoices(ctx context.Context, modelID string) (*VoicesResponse, error) {
	path := fmt.Sprintf("/v1/models/%s/voices", url.PathEscape(modelID))
	var result VoicesResponse
	err := c.doJSONRequest(ctx, http.MethodGet, path, nil, &result)
	return &result, err
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, contentType string) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		switch v := body.(type) {
		case io.Reader:
			bodyReader = v
		default:
			jsonBytes, err := json.Marshal(body)
			if err != nil {
				return nil, NewError(ErrInternal, "failed to marshal request body", err.Error())
			}
			bodyReader = bytes.NewReader(jsonBytes)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, NewError(ErrInternal, "failed to create request", err.Error())
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, mapHTTPError(err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, parseErrorResponse(resp.StatusCode, respBytes)
	}

	return resp, nil
}

func (c *Client) doJSONRequest(ctx context.Context, method, path string, reqBody, respBody interface{}) error {
	resp, err := c.doRequest(ctx, method, path, reqBody, "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return NewError(ErrInternal, "failed to read response body", err.Error())
	}

	if respBody != nil {
		if err := json.Unmarshal(bodyBytes, respBody); err != nil {
			return NewError(ErrInternal, "failed to parse response JSON", err.Error())
		}
	}
	return nil
}

func mapHTTPError(err error) *Error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return NewError(ErrRequestTimeout, "request timed out", urlErr.Error())
		}
		var opErr *net.OpError
		if errors.As(urlErr.Err, &opErr) {
			if opErr.Op == "dial" {
				return NewError(ErrServerUnavailable, "connection refused", urlErr.Error())
			}
			return NewError(ErrServerUnavailable, "network error", opErr.Error())
		}
		return NewError(ErrServerUnavailable, "server unavailable", urlErr.Error())
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return NewError(ErrRequestTimeout, "request cancelled", err.Error())
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return NewError(ErrServerUnavailable, "network error", netErr.Error())
	}

	return NewError(ErrInternal, "unexpected error", err.Error())
}

func parseErrorResponse(statusCode int, body []byte) *Error {
	var serverErr struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &serverErr) == nil && serverErr.Error.Message != "" {
		return NewError(mapServerErrorType(serverErr.Error.Type), serverErr.Error.Message, nil)
	}

	switch statusCode {
	case http.StatusNotFound:
		return NewError(ErrModelNotFound, "resource not found", nil)
	case http.StatusBadRequest:
		return NewError(ErrInvalidRequest, "invalid request", nil)
	case http.StatusServiceUnavailable:
		return NewError(ErrServerUnavailable, "server unavailable", nil)
	default:
		return NewError(ErrInternal, fmt.Sprintf("unexpected status %d", statusCode), string(body))
	}
}

func mapServerErrorType(typ string) string {
	switch typ {
	case "model_not_found":
		return ErrModelNotFound
	case "invalid_request":
		return ErrInvalidRequest
	case "timeout":
		return ErrRequestTimeout
	default:
		return ErrInternal
	}
}

type HealthResponse struct {
	Status  string `json:"status"`
	Backend string `json:"backend"`
	Models  int    `json:"models"`
}

type ModelsResponse struct {
	Object string      `json:"object"`
	Data   []ModelInfo `json:"data"`
}

type ModelInfo struct {
	ID     string `json:"id"`
	Object string `json:"object"`
	Family string `json:"family"`
	Task   string `json:"task"`
	Mode   string `json:"mode"`
}

type SpeechRequest struct {
	Model             string            `json:"model"`
	Input             string            `json:"input"`
	Voice             string            `json:"voice,omitempty"`
	VoiceRef          string            `json:"voice_ref,omitempty"`
	ReferenceText     string            `json:"reference_text,omitempty"`
	Language          string            `json:"language,omitempty"`
	Seed              int               `json:"seed,omitempty"`
	Temperature       float64           `json:"temperature,omitempty"`
	TopK              int               `json:"top_k,omitempty"`
	TopP              float64           `json:"top_p,omitempty"`
	MaxTokens         int               `json:"max_tokens,omitempty"`
	RepetitionPenalty float64           `json:"repetition_penalty,omitempty"`
	GuidanceScale     float64           `json:"guidance_scale,omitempty"`
	NumInferenceSteps int               `json:"num_inference_steps,omitempty"`
	Options           map[string]string `json:"options,omitempty"`
	ResponseFormat    string            `json:"response_format,omitempty"`
}

type TranscribeRequest struct {
	Model    string `json:"model"`
	Audio    string `json:"audio"`
	Language string `json:"language,omitempty"`
	Context  string `json:"text,omitempty"`
	Stream   bool   `json:"stream,omitempty"`
}

type TranscribeResponse struct {
	Text   string      `json:"text"`
	Timing *TimingInfo `json:"timing,omitempty"`
}

type TaskRequest struct {
	Model   string                 `json:"model"`
	Request map[string]interface{} `json:"request"`
}

type TaskResponse struct {
	Text              string          `json:"text,omitempty"`
	Audio             string          `json:"audio,omitempty"`
	SampleRate        int             `json:"sample_rate,omitempty"`
	Channels          int             `json:"channels,omitempty"`
	NamedAudioOutputs []NamedAudio    `json:"named_audio_outputs,omitempty"`
	Segments          []Segment       `json:"segments,omitempty"`
	SpeakerTurns      []SpeakerTurn   `json:"speaker_turns,omitempty"`
	Words             []WordTimestamp `json:"words,omitempty"`
	Timing            *TimingInfo     `json:"timing,omitempty"`
}

type NamedAudio struct {
	ID         string `json:"id"`
	Audio      string `json:"audio"`
	SampleRate int    `json:"sample_rate"`
	Channels   int    `json:"channels"`
}

type Segment struct {
	StartSample int     `json:"start_sample"`
	EndSample   int     `json:"end_sample"`
	Confidence  float64 `json:"confidence"`
}

type SpeakerTurn struct {
	StartSample int     `json:"start_sample"`
	EndSample   int     `json:"end_sample"`
	SpeakerID   string  `json:"speaker_id"`
	Confidence  float64 `json:"confidence"`
}

type WordTimestamp struct {
	Word        string  `json:"word"`
	StartSample int     `json:"start_sample"`
	EndSample   int     `json:"end_sample"`
	Confidence  float64 `json:"confidence"`
}

type TimingInfo struct {
	WallMs          float64 `json:"wall_ms,omitempty"`
	AudioDurationMs float64 `json:"audio_duration_ms,omitempty"`
	Rtf             float64 `json:"rtf,omitempty"`
	TtftMs          float64 `json:"ttft_ms,omitempty"`
}

type VoicesResponse struct {
	Voices []string `json:"voices"`
}
