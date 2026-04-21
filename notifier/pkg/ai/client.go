package ai

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/runtime-radar/runtime-radar/notifier/pkg/model"
)

const (
	defaultTimeout       = 45 * time.Second
	defaultMaxTokens     = 400
	defaultOpenAIBaseURL = "https://api.openai.com/v1"
	defaultAnthropicURL  = "https://api.anthropic.com/v1"
	defaultOllamaBaseURL = "http://localhost:11434"
	anthropicVersion     = "2023-06-01"
	maxEventJSONBytes    = 32 * 1024
	explainSystemPrompt  = "You are a security analyst for runtime security events. Explain the event briefly and return JSON only with keys: summary, risk, possible_cause, next_steps. next_steps must be an array of short strings."
	testPrompt           = "Reply with JSON only: {\"summary\":\"ok\",\"risk\":\"info\",\"possible_cause\":\"connection ok\",\"next_steps\":[\"none\"]}"
)

type Client interface {
	Test(context.Context) error
	ExplainRuntimeEvent(ctx context.Context, eventID, eventJSON string) (*Result, error)
}

type Result struct {
	Summary       string
	Risk          string
	PossibleCause string
	NextSteps     []string
	RawText       string
}

type parsedResult struct {
	Summary       string   `json:"summary"`
	Risk          string   `json:"risk"`
	PossibleCause string   `json:"possible_cause"`
	NextSteps     []string `json:"next_steps"`
}

type baseClient struct {
	conf       *model.AI
	httpClient *http.Client
	baseURL    string
}

func NewClient(conf *model.AI) (Client, error) {
	httpClient, err := newHTTPClient(conf)
	if err != nil {
		return nil, err
	}

	base := &baseClient{
		conf:       conf,
		httpClient: httpClient,
		baseURL:    resolveBaseURL(conf),
	}

	switch conf.Provider {
	case model.AIProviderOpenAICompatible:
		return &openAICompatibleClient{baseClient: base}, nil
	case model.AIProviderAnthropic:
		return &anthropicClient{baseClient: base}, nil
	case model.AIProviderOllama:
		return &ollamaClient{baseClient: base}, nil
	default:
		return nil, fmt.Errorf("unsupported ai provider: %s", conf.Provider)
	}
}

func newHTTPClient(conf *model.AI) (*http.Client, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if conf.Insecure {
		tlsConfig.InsecureSkipVerify = true
	}

	if conf.CA != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(conf.CA)) {
			return nil, fmt.Errorf("can't parse custom ca")
		}

		tlsConfig.RootCAs = pool
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig

	return &http.Client{
		Timeout:   defaultTimeout,
		Transport: transport,
	}, nil
}

func resolveBaseURL(conf *model.AI) string {
	if conf.BaseURL != "" {
		return strings.TrimRight(conf.BaseURL, "/")
	}

	switch conf.Provider {
	case model.AIProviderAnthropic:
		return defaultAnthropicURL
	case model.AIProviderOllama:
		return defaultOllamaBaseURL
	default:
		return defaultOpenAIBaseURL
	}
}

func buildExplainUserPrompt(eventID, eventJSON string) string {
	eventJSON = trimEventJSON(eventJSON)

	return fmt.Sprintf("Explain this runtime event. Event ID: %s\nEvent JSON:\n%s", eventID, eventJSON)
}

func trimEventJSON(eventJSON string) string {
	if len(eventJSON) <= maxEventJSONBytes {
		return eventJSON
	}

	return eventJSON[:maxEventJSONBytes]
}

func parseResult(raw string) *Result {
	result := &Result{
		Summary: strings.TrimSpace(raw),
		RawText: raw,
	}

	candidate := strings.TrimSpace(raw)
	candidate = strings.TrimPrefix(candidate, "```json")
	candidate = strings.TrimPrefix(candidate, "```")
	candidate = strings.TrimSuffix(candidate, "```")
	candidate = strings.TrimSpace(candidate)

	var parsed parsedResult
	if err := json.Unmarshal([]byte(candidate), &parsed); err != nil {
		return result
	}

	result.Summary = parsed.Summary
	result.Risk = parsed.Risk
	result.PossibleCause = parsed.PossibleCause
	result.NextSteps = parsed.NextSteps

	return result
}

func newJSONRequest(ctx context.Context, method, url string, body any) (*http.Request, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

func readResponse(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return body, nil
}

type openAICompatibleClient struct {
	*baseClient
}

func (c *openAICompatibleClient) Test(ctx context.Context) error {
	_, err := c.complete(ctx, testPrompt, 32)
	return err
}

func (c *openAICompatibleClient) ExplainRuntimeEvent(ctx context.Context, eventID, eventJSON string) (*Result, error) {
	raw, err := c.complete(ctx, buildExplainUserPrompt(eventID, eventJSON), defaultMaxTokens)
	if err != nil {
		return nil, err
	}

	return parseResult(raw), nil
}

func (c *openAICompatibleClient) complete(ctx context.Context, userPrompt string, maxTokens int) (string, error) {
	reqBody := map[string]any{
		"model": c.conf.Model,
		"messages": []map[string]string{
			{"role": "system", "content": explainSystemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.1,
		"max_tokens":  maxTokens,
	}

	req, err := newJSONRequest(ctx, http.MethodPost, c.baseURL+"/chat/completions", reqBody)
	if err != nil {
		return "", err
	}

	if c.conf.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.conf.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}

	body, err := readResponse(resp)
	if err != nil {
		return "", err
	}

	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if len(payload.Choices) == 0 {
		return "", fmt.Errorf("empty response choices")
	}

	return payload.Choices[0].Message.Content, nil
}

type anthropicClient struct {
	*baseClient
}

func (c *anthropicClient) Test(ctx context.Context) error {
	_, err := c.complete(ctx, testPrompt, 32)
	return err
}

func (c *anthropicClient) ExplainRuntimeEvent(ctx context.Context, eventID, eventJSON string) (*Result, error) {
	raw, err := c.complete(ctx, buildExplainUserPrompt(eventID, eventJSON), defaultMaxTokens)
	if err != nil {
		return nil, err
	}

	return parseResult(raw), nil
}

func (c *anthropicClient) complete(ctx context.Context, userPrompt string, maxTokens int) (string, error) {
	reqBody := map[string]any{
		"model":       c.conf.Model,
		"system":      explainSystemPrompt,
		"max_tokens":  maxTokens,
		"temperature": 0.1,
		"messages": []map[string]string{
			{"role": "user", "content": userPrompt},
		},
	}

	req, err := newJSONRequest(ctx, http.MethodPost, c.baseURL+"/messages", reqBody)
	if err != nil {
		return "", err
	}

	if c.conf.APIKey != "" {
		req.Header.Set("x-api-key", c.conf.APIKey)
	}
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}

	body, err := readResponse(resp)
	if err != nil {
		return "", err
	}

	var payload struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if len(payload.Content) == 0 {
		return "", fmt.Errorf("empty response content")
	}

	parts := make([]string, 0, len(payload.Content))
	for _, item := range payload.Content {
		if item.Type == "text" {
			parts = append(parts, item.Text)
		}
	}

	return strings.Join(parts, "\n"), nil
}

type ollamaClient struct {
	*baseClient
}

func (c *ollamaClient) Test(ctx context.Context) error {
	_, err := c.complete(ctx, testPrompt, 32)
	return err
}

func (c *ollamaClient) ExplainRuntimeEvent(ctx context.Context, eventID, eventJSON string) (*Result, error) {
	raw, err := c.complete(ctx, buildExplainUserPrompt(eventID, eventJSON), defaultMaxTokens)
	if err != nil {
		return nil, err
	}

	return parseResult(raw), nil
}

func (c *ollamaClient) complete(ctx context.Context, userPrompt string, maxTokens int) (string, error) {
	reqBody := map[string]any{
		"model": c.conf.Model,
		"messages": []map[string]string{
			{"role": "system", "content": explainSystemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"stream": false,
		"options": map[string]any{
			"temperature": 0.1,
			"num_predict": maxTokens,
		},
	}

	req, err := newJSONRequest(ctx, http.MethodPost, c.baseURL+"/api/chat", reqBody)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}

	body, err := readResponse(resp)
	if err != nil {
		return "", err
	}

	var payload struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if payload.Message.Content == "" {
		return "", fmt.Errorf("empty response content")
	}

	return payload.Message.Content, nil
}
