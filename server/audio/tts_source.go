package audio

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type RequestBody struct {
	Text      string `json:"text"`
	Speaker   string `json:"speaker"`
	Translate bool   `json:"translate"`
}

type InvokeResponse struct {
	Code string `json:"code"`

	SKey     string `json:"s_key"`
	VStr     string `json:"v_str"`
	Duration string `json:"duration"`
	Speaker  string `json:"speaker"`
}

var (
	client = &http.Client{
		Timeout: 10 * time.Second,
	}
)

func NewTTSSource(ctx context.Context, urlStr string, startTimeMs int64) (*MP3Source, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}

	query := parsedURL.Query()
	reqBody := RequestBody{
		Text:      query.Get("text"),
		Speaker:   query.Get("speaker"),
		Translate: query.Get("translate") == "true",
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("encode req body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", config.TextToSpeechURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", config.UserAgent)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch tts: %w", err)
	}
	defer resp.Body.Close()

	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		return nil, fmt.Errorf("content type")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var data InvokeResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse tts response: %w", err)
	}

	if len(data.Code) > 0 {
		return nil, errors.New(data.Code)
	}

	audioBytes, err := base64.StdEncoding.DecodeString(data.VStr)
	if err != nil {
		return nil, fmt.Errorf("decode base64 audio: %w", err)
	}

	if len(audioBytes) == 0 {
		return nil, errors.New("empty audio data")
	}

	reader := io.NopCloser(bytes.NewReader(audioBytes))
	source, err := NewMP3SourceFromReader(reader, urlStr, startTimeMs)
	if err != nil {
		return nil, err
	}

	return source, nil
}
