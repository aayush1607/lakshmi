// Package llm — Azure AI Foundry client.
//
// Foundry exposes OpenAI's chat-completion schema at a deployment URL:
//
//	POST {endpoint}/openai/deployments/{deployment}/chat/completions
//	     ?api-version={api_version}
//	Headers: api-key: <AZURE_FOUNDRY_API_KEY>
//
// We do not use the official Azure SDK: the surface we need is tiny
// (one POST, a few fields) and pulling in the SDK drags in half of
// the Azure ecosystem for no gain. The hand-rolled client is also
// trivially fakeable in tests.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AzureConfig is the deployment-level config for a Foundry client.
type AzureConfig struct {
	Endpoint   string // https://<resource>.openai.azure.com OR a project-level URL
	Deployment string // the deployed model name, e.g. "gpt-4o-mini"
	APIKey     string // from AZURE_FOUNDRY_API_KEY
	APIVersion string // e.g. "2024-10-21"; defaults to "2024-10-21" when empty
	HTTPClient *http.Client
}

// NewAzure builds a client from config. The caller owns secret handling —
// this code never writes the key to disk or logs.
func NewAzure(cfg AzureConfig) *AzureClient {
	if cfg.APIVersion == "" {
		cfg.APIVersion = "2024-10-21"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &AzureClient{cfg: cfg}
}

// AzureClient implements Client.
type AzureClient struct {
	cfg AzureConfig
}

// Complete sends a chat-completion request.
func (c *AzureClient) Complete(ctx context.Context, req Request) (Response, error) {
	if c.cfg.Endpoint == "" || c.cfg.Deployment == "" || c.cfg.APIKey == "" {
		return Response{}, fmt.Errorf("azure foundry: endpoint, deployment, and api key are all required")
	}

	body := map[string]any{
		"messages":    buildMessages(req),
		"temperature": req.Temperature,
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if req.JSONSchema != nil {
		name := req.SchemaName
		if name == "" {
			name = "Output"
		}
		body["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   name,
				"strict": true,
				"schema": req.JSONSchema,
			},
		}
	}

	buf, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}

	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		strings.TrimRight(c.cfg.Endpoint, "/"),
		c.cfg.Deployment,
		c.cfg.APIVersion,
	)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", c.cfg.APIKey)

	resp, err := c.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("azure foundry: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}
	if resp.StatusCode >= 400 {
		return Response{}, fmt.Errorf("azure foundry: http %d: %s", resp.StatusCode, truncate(string(raw), 400))
	}

	var parsed azureResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Response{}, fmt.Errorf("azure foundry: decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return Response{}, fmt.Errorf("azure foundry: empty choices")
	}
	return Response{
		Content: parsed.Choices[0].Message.Content,
		Usage: Usage{
			PromptTokens:     parsed.Usage.PromptTokens,
			CompletionTokens: parsed.Usage.CompletionTokens,
			TotalTokens:      parsed.Usage.TotalTokens,
		},
		Raw: json.RawMessage(raw),
	}, nil
}

func buildMessages(req Request) []map[string]string {
	msgs := make([]map[string]string, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]string{"role": string(m.Role), "content": m.Content})
	}
	return msgs
}

type azureResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
