// Package llm calls an OpenAI-compatible /chat/completions API (OpenAI, a router, a local
// server) for answers shaped by a JSON schema.
package llm

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

// Turn is one chat message; Role is "user" or "model" (the app's earlier answer).
type Turn struct {
	Role string
	Text string
}

type Client struct {
	endpoint, key, model string
	client               *http.Client
}

// New takes the API base URL, e.g. "https://api.openai.com/v1".
func New(endpoint, apiKey, model string) *Client {
	return &Client{endpoint: strings.TrimRight(endpoint, "/"), key: apiKey, model: model, client: &http.Client{Timeout: 60 * time.Second}}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// JSON asks for an answer matching schema (JSON Schema) and returns its raw text.
func (c *Client) JSON(ctx context.Context, system string, turns []Turn, schema any) (string, error) {
	messages := []message{{"system", system}}
	for _, t := range turns {
		role := t.Role
		if role == "model" {
			role = "assistant"
		}
		messages = append(messages, message{role, t.Text})
	}
	body, _ := json.Marshal(map[string]any{
		"model":       c.model,
		"messages":    messages,
		"temperature": 0,
		"stream":      false,
		"max_tokens":  2000, // a draft is small; this caps the cost of a runaway answer
		"response_format": map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": "answer", "schema": schema},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return "", fmt.Errorf("llm: %s: %s", res.Status, msg)
	}
	var out struct {
		Choices []struct {
			Message message `json:"message"`
		} `json:"choices"`
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("llm: decode: %w: %.200s", err, raw)
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == "" {
		return "", errors.New("llm: empty answer")
	}
	// Some models wrap JSON in a ```json fence even when asked for JSON.
	text := strings.TrimSpace(out.Choices[0].Message.Content)
	text = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(text, "```json"), "```"), "```")
	return strings.TrimSpace(text), nil
}
