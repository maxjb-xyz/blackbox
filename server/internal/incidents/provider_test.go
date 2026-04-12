package incidents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOllamaProvider_Generate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/generate", r.URL.Path)
		var req ollamaRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "llama3.2", req.Model)
		require.Equal(t, "test prompt", req.Prompt)
		require.False(t, req.Stream)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ollamaResponse{Response: "test result"})
	}))
	defer srv.Close()

	p := &ollamaProvider{baseURL: srv.URL}
	result, err := p.Generate(context.Background(), "llama3.2", "test prompt", 5*time.Second)
	require.NoError(t, err)
	require.Equal(t, "test result", result)
}

func TestOpenAICompatProvider_Generate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		var req openAIRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "gpt-4o-mini", req.Model)
		require.False(t, req.Stream)
		require.Len(t, req.Messages, 1)
		require.Equal(t, "user", req.Messages[0].Role)
		require.Equal(t, "test prompt", req.Messages[0].Content)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openAIResponse{
			Choices: []openAIChoice{{Message: openAIMessage{Role: "assistant", Content: "openai result"}}},
		})
	}))
	defer srv.Close()

	p := &openAICompatProvider{baseURL: srv.URL, apiKey: "sk-test"}
	result, err := p.Generate(context.Background(), "gpt-4o-mini", "test prompt", 5*time.Second)
	require.NoError(t, err)
	require.Equal(t, "openai result", result)
}

func TestOpenAICompatProvider_Generate_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	p := &openAICompatProvider{baseURL: srv.URL, apiKey: "bad-key"}
	_, err := p.Generate(context.Background(), "gpt-4o-mini", "prompt", 5*time.Second)
	require.Error(t, err)
	require.Contains(t, err.Error(), "401")
}

func TestOpenAICompatProvider_Generate_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openAIResponse{Choices: nil})
	}))
	defer srv.Close()

	p := &openAICompatProvider{baseURL: srv.URL, apiKey: ""}
	_, err := p.Generate(context.Background(), "gpt-4o-mini", "prompt", 5*time.Second)
	require.EqualError(t, err, "openai response missing choices")
}

func TestOpenAICompatProvider_Generate_NoAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIResponse{
			Choices: []openAIChoice{{Message: openAIMessage{Role: "assistant", Content: "no-auth result"}}},
		})
	}))
	defer srv.Close()

	p := &openAICompatProvider{baseURL: srv.URL, apiKey: ""}
	result, err := p.Generate(context.Background(), "model", "prompt", 5*time.Second)
	require.NoError(t, err)
	require.Equal(t, "no-auth result", result)
}
