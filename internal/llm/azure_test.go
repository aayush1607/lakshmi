package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAzureCompleteHappy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "secret" {
			t.Errorf("missing api-key")
		}
		if r.URL.Query().Get("api-version") != "2024-10-21" {
			t.Errorf("api-version = %q", r.URL.Query().Get("api-version"))
		}
		if !strings.Contains(r.URL.Path, "/deployments/gpt-4o-mini/chat/completions") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if _, ok := body["response_format"]; !ok {
			t.Error("response_format not passed through")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"}}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`))
	}))
	defer srv.Close()

	c := NewAzure(AzureConfig{
		Endpoint:   srv.URL,
		Deployment: "gpt-4o-mini",
		APIKey:     "secret",
	})
	resp, err := c.Complete(context.Background(), Request{
		System:     "be grounded",
		Messages:   []Message{{Role: RoleUser, Content: "hi"}},
		JSONSchema: map[string]any{"type": "object"},
		SchemaName: "Reply",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != `{"ok":true}` {
		t.Fatalf("content = %q", resp.Content)
	}
	if resp.Usage.TotalTokens != 12 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
}

func TestAzureCompleteErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer srv.Close()
	c := NewAzure(AzureConfig{Endpoint: srv.URL, Deployment: "d", APIKey: "k"})
	_, err := c.Complete(context.Background(), Request{})
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("want 429 error, got %v", err)
	}
}

func TestAzureMissingConfig(t *testing.T) {
	c := NewAzure(AzureConfig{})
	_, err := c.Complete(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}
