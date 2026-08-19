package ai

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/runtime-radar/runtime-radar/notifier/pkg/model"
)

const (
	defaultTimeout = 5 * time.Minute
	// A local model may legitimately think for minutes, but an operator waiting
	// on a "check connection" button must not, so the probe caps itself.
	testTimeout          = 15 * time.Second
	defaultMaxTokens     = 1024
	defaultOpenAIBaseURL = "https://api.openai.com/v1"
	openAIAPIHost        = "api.openai.com"
	defaultAnthropicURL  = "https://api.anthropic.com/v1"
	defaultOllamaBaseURL = "http://localhost:11434"
	anthropicVersion     = "2023-06-01"
	maxEventJSONBytes    = 32 * 1024
	maxErrorTextBytes    = 200
	truncationMarker     = "\n...[truncated]"
	// Enough for testPrompt's answer with headroom: a reply cut off mid-JSON
	// would fail the parse check below for no reason. Sized for the Cyrillic
	// answer explainSystemPrompt asks for, which many tokenizers spend two to
	// three tokens per character on.
	testMaxTokens       = 256
	explainSystemPrompt = "You are a security analyst for runtime security events. Explain the event briefly in Russian and return JSON only with keys: summary, risk, possible_cause, next_steps. All string values must be in Russian. next_steps must be an array of short strings."
	testPrompt          = "Reply with JSON only: {\"summary\":\"ok\",\"risk\":\"info\",\"possible_cause\":\"connection ok\",\"next_steps\":[\"none\"]}"
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
	// Re-checked per request rather than trusted from validation alone: a stored
	// config may predate this check, and create/update can skip checks entirely.
	if conf.IsLocal && !IsLocalEndpoint(conf) {
		return nil, fmt.Errorf("integration is marked local but its endpoint is not")
	}

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

// Name suffixes that only an internal resolver can answer.
var localHostSuffixes = []string{".local", ".localdomain", ".internal", ".svc"}

// IsLocalEndpoint reports whether the endpoint given conf resolves to is served
// from inside the deployment: a loopback, private or link-local address, or a
// name only an internal resolver can answer. A public name is rejected even
// when it currently points at a private address, because establishing that
// needs a lookup which may answer differently by the time a request is sent.
func IsLocalEndpoint(conf *model.AI) bool {
	parsed, err := url.Parse(resolveBaseURL(conf))
	if err != nil {
		return false
	}

	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" {
		return false
	}

	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}

	// A single label is only resolvable internally, which is how in-cluster
	// services are usually addressed: http://ollama:11434.
	if host == "localhost" || !strings.Contains(host, ".") {
		return true
	}

	for _, suffix := range localHostSuffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}

	return false
}

// isOfficialOpenAI reports whether given base URL points at OpenAI's own API,
// which rejects request arguments it doesn't know about.
func isOfficialOpenAI(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}

	return strings.EqualFold(parsed.Hostname(), openAIAPIHost)
}

func buildExplainUserPrompt(eventID, eventJSON string) string {
	eventJSON = trimEventJSON(eventJSON)

	return fmt.Sprintf("Explain this runtime event. Event ID: %s\nEvent JSON:\n%s", eventID, eventJSON)
}

func trimEventJSON(eventJSON string) string {
	if len(eventJSON) <= maxEventJSONBytes {
		return eventJSON
	}

	// Cutting mid-rune leaves bytes that json.Marshal silently rewrites to
	// U+FFFD, and cutting at all leaves the model holding malformed JSON, so
	// back off to a rune boundary and say outright that the document is partial.
	trimmed := eventJSON[:maxEventJSONBytes]
	for len(trimmed) > 0 {
		// A real U+FFFD in the input decodes with size 3; a broken tail with 1.
		if r, size := utf8.DecodeLastRuneInString(trimmed); r != utf8.RuneError || size > 1 {
			break
		}

		trimmed = trimmed[:len(trimmed)-1]
	}

	return trimmed + truncationMarker
}

// parseResult extracts the structured answer out of raw model output. The bool
// reports whether raw actually held the JSON object the prompt asked for; when
// it didn't, only RawText is filled in, so that callers never present unparsed
// model output (a stray preamble, or a chain of thought) as an analyst summary.
func parseResult(raw string) (*Result, bool) {
	result := &Result{RawText: raw}

	candidate := strings.TrimSpace(raw)
	candidate = strings.TrimPrefix(candidate, "```json")
	candidate = strings.TrimPrefix(candidate, "```")
	candidate = strings.TrimSuffix(candidate, "```")
	candidate = strings.TrimSpace(candidate)

	var parsed parsedResult
	if err := json.Unmarshal([]byte(candidate), &parsed); err != nil {
		return result, false
	}

	result.Summary = parsed.Summary
	result.Risk = parsed.Risk
	result.PossibleCause = parsed.PossibleCause
	result.NextSteps = parsed.NextSteps

	return result, true
}

type completeFunc func(ctx context.Context, userPrompt string, maxTokens int) (string, error)

// runTest performs the connectivity probe with a deadline of its own, on top of
// whatever the caller already imposed.
func (c *baseClient) runTest(ctx context.Context, complete completeFunc) error {
	ctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()

	return checkTestResponse(complete(ctx, testPrompt, testMaxTokens))
}

// checkTestResponse turns a completion into a connectivity verdict. A reachable
// endpoint whose answer can't be parsed still can't explain anything, so the
// check fails here instead of on the operator's first real request.
func checkTestResponse(raw string, err error) error {
	if err != nil {
		return err
	}

	if _, ok := parseResult(raw); !ok {
		return fmt.Errorf("model response is not the expected JSON: %s", truncateForError(raw))
	}

	return nil
}

func truncateForError(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= maxErrorTextBytes {
		return text
	}

	return text[:maxErrorTextBytes] + "..."
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
	return c.runTest(ctx, c.complete)
}

func (c *openAICompatibleClient) ExplainRuntimeEvent(ctx context.Context, eventID, eventJSON string) (*Result, error) {
	raw, err := c.complete(ctx, buildExplainUserPrompt(eventID, eventJSON), defaultMaxTokens)
	if err != nil {
		return nil, err
	}

	result, _ := parseResult(raw)

	return result, nil
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

	// Qwen3/vLLM reasoning models otherwise spend the whole budget on thinking
	// and return empty message.content. It's a self-hosted extension, so it's
	// only sent to compatible backends: OpenAI itself rejects unknown arguments.
	if !isOfficialOpenAI(c.baseURL) {
		reqBody["chat_template_kwargs"] = map[string]any{
			"enable_thinking": false,
		}
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
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if len(payload.Choices) == 0 {
		return "", fmt.Errorf("empty response choices")
	}

	content := strings.TrimSpace(payload.Choices[0].Message.Content)
	if content == "" {
		content = strings.TrimSpace(payload.Choices[0].Message.ReasoningContent)
	}
	if content == "" {
		return "", fmt.Errorf("empty response content")
	}

	return content, nil
}

type anthropicClient struct {
	*baseClient
}

func (c *anthropicClient) Test(ctx context.Context) error {
	return c.runTest(ctx, c.complete)
}

func (c *anthropicClient) ExplainRuntimeEvent(ctx context.Context, eventID, eventJSON string) (*Result, error) {
	raw, err := c.complete(ctx, buildExplainUserPrompt(eventID, eventJSON), defaultMaxTokens)
	if err != nil {
		return nil, err
	}

	result, _ := parseResult(raw)

	return result, nil
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
	parts := make([]string, 0, len(payload.Content))
	for _, item := range payload.Content {
		if item.Type == "text" {
			parts = append(parts, item.Text)
		}
	}

	// Not the same as len(Content) == 0: a thinking-enabled model answers with
	// blocks that carry no text at all, which is just as unusable.
	content := strings.TrimSpace(strings.Join(parts, "\n"))
	if content == "" {
		return "", fmt.Errorf("empty response content")
	}

	return content, nil
}

type ollamaClient struct {
	*baseClient
}

func (c *ollamaClient) Test(ctx context.Context) error {
	return c.runTest(ctx, c.complete)
}

func (c *ollamaClient) ExplainRuntimeEvent(ctx context.Context, eventID, eventJSON string) (*Result, error) {
	raw, err := c.complete(ctx, buildExplainUserPrompt(eventID, eventJSON), defaultMaxTokens)
	if err != nil {
		return nil, err
	}

	result, _ := parseResult(raw)

	return result, nil
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

	// Ollama itself ignores this, but it's commonly fronted by a proxy that
	// doesn't, and the form offers an API key field for every provider.
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
