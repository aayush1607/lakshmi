// Package llm is Lakshmi's abstraction over chat-completion LLMs.
//
// Design:
//   - One interface (Client) that hides provider specifics.
//   - One concrete impl in v1: Azure AI Foundry via its OpenAI-compatible
//     REST API (see azure.go).
//   - JSON-mode-only for the Reason step so the agent can validate
//     responses deterministically. No raw free-text from the model
//     touches the user directly — everything flows through typed fields.
//   - No streaming in v1. Final answer only. Streaming is a polish that
//     lands when/if we do conversational latency work.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Role is a chat-message role. We use the OpenAI-style strings because
// Azure Foundry speaks OpenAI's schema verbatim.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is a single chat turn.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// Request is the input to a Complete call.
type Request struct {
	System      string    // becomes the first system message
	Messages    []Message // ordered conversation turns (user/assistant)
	MaxTokens   int       // 0 → provider default
	Temperature float32   // 0 → provider default (treated as 0 for determinism in v1)

	// JSONSchema, if set, forces the response to match this schema via
	// Azure's `response_format: {"type":"json_schema",...}`. The value
	// must be a JSON-marshalable schema object.
	JSONSchema any
	// SchemaName is a short identifier required by Azure when JSONSchema
	// is set (e.g. "ReasonOutput").
	SchemaName string
}

// Response is what a Complete call returns.
type Response struct {
	Content string      // assistant message content; with JSON mode this is parseable JSON
	Usage   Usage       // token counts; zero-valued if the provider did not return it
	Raw     interface{} // provider-native response body for /why debugging
}

// Usage mirrors the OpenAI usage shape.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Client is the abstraction the agent talks to.
type Client interface {
	// Complete performs a chat-completion call.
	Complete(ctx context.Context, req Request) (Response, error)
}

// DecodeJSON unmarshals Response.Content into v. Helper so agent code
// doesn't repeat the boilerplate.
//
// Some models (notably reasoning models like DeepSeek-R1, and any model
// invoked on a deployment that doesn't honor response_format) wrap the
// JSON in prose, ```json fences, or <think>…</think> blocks. We strip
// the obvious envelopes and then scan for the first balanced top-level
// JSON object. Strict json_schema deployments still parse on the fast
// path because their content is already pure JSON.
func DecodeJSON(resp Response, v any) error {
	raw := strings.TrimSpace(resp.Content)
	if err := json.Unmarshal([]byte(raw), v); err == nil {
		return nil
	}
	cleaned := extractJSON(raw)
	if cleaned == "" {
		return fmt.Errorf("no JSON object found in model response (got %d chars of prose)", len(raw))
	}
	return json.Unmarshal([]byte(cleaned), v)
}

// extractJSON pulls the first balanced {...} block out of s, ignoring
// braces that appear inside string literals. Returns "" if no balanced
// object is found. Conservative on purpose — does not try to repair
// malformed JSON, only to find it.
func extractJSON(s string) string {
	// Strip <think>…</think> reasoning blocks if present.
	for {
		i := strings.Index(s, "<think>")
		if i < 0 {
			break
		}
		j := strings.Index(s[i:], "</think>")
		if j < 0 {
			s = s[:i]
			break
		}
		s = s[:i] + s[i+j+len("</think>"):]
	}
	// Strip ```json … ``` fences.
	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i+3:]
		rest = strings.TrimPrefix(rest, "json")
		rest = strings.TrimPrefix(rest, "JSON")
		if j := strings.Index(rest, "```"); j >= 0 {
			s = rest[:j]
		}
	}
	// Find first balanced top-level object.
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			switch c {
			case '\\':
				esc = true
			case '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}
