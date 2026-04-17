package incidents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LLMProvider abstracts the HTTP call to an AI backend.
type LLMProvider interface {
	Generate(ctx context.Context, model, prompt string, timeout time.Duration) (string, error)
}

// ollamaProvider calls the Ollama /api/generate endpoint.
type ollamaProvider struct {
	baseURL string
}

func (p *ollamaProvider) Generate(ctx context.Context, model, prompt string, timeout time.Duration) (string, error) {
	return callOllamaWithTimeout(ctx, p.baseURL, model, prompt, timeout)
}

// openAICompatProvider calls any OpenAI-compatible /v1/chat/completions endpoint.
type openAICompatProvider struct {
	baseURL string
	apiKey  string
}

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChoice struct {
	Message openAIMessage `json:"message"`
}

type openAIResponse struct {
	Choices []openAIChoice `json:"choices"`
}

// GenerateWithConfig instantiates an AI provider from raw settings and runs a
// single prompt through it.
func GenerateWithConfig(ctx context.Context, providerType, baseURL, model, apiKey, prompt string, timeout time.Duration) (string, error) {
	providerType = strings.TrimSpace(providerType)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	model = strings.TrimSpace(model)
	if baseURL == "" {
		return "", errors.New("baseURL is required")
	}
	if model == "" {
		return "", errors.New("model is required")
	}

	var provider LLMProvider
	switch providerType {
	case "ollama":
		provider = &ollamaProvider{baseURL: baseURL}
	case "openai_compat":
		provider = &openAICompatProvider{baseURL: baseURL, apiKey: strings.TrimSpace(apiKey)}
	default:
		return "", fmt.Errorf("unsupported ai provider %q", providerType)
	}

	return provider.Generate(ctx, model, prompt, timeout)
}

func (p *openAICompatProvider) Generate(ctx context.Context, model, prompt string, timeout time.Duration) (string, error) {
	reqBody, _ := json.Marshal(openAIRequest{
		Model:    model,
		Messages: []openAIMessage{{Role: "user", Content: prompt}},
		Stream:   false,
	})
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("openai responded with %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var result openAIResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", errors.New("openai response missing choices")
	}
	content := strings.TrimSpace(result.Choices[0].Message.Content)
	if content == "" {
		return "", errors.New("openai response was empty")
	}
	return content, nil
}
