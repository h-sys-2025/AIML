package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OllamaClient struct {
	host    string
	model   string
	client  *http.Client
	options map[string]interface{}
	raw     bool
	think   *bool
}

func NewOllamaClient(host, model string) *OllamaClient {
	return &OllamaClient{
		host:  host,
		model: model,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
		options: map[string]interface{}{
			"temperature": 0.7,
			"num_predict": 1024,
			"num_ctx":     4096,
		},
	}
}

func (c *OllamaClient) SetRaw(v bool)   { c.raw = v }
func (c *OllamaClient) Raw() bool       { return c.raw }
func (c *OllamaClient) SetThink(v bool) { c.think = &v }
func (c *OllamaClient) ClearThink()     { c.think = nil }
func (c *OllamaClient) GetThink() *bool { return c.think }

func (c *OllamaClient) SetModel(model string) {
	c.model = model
}

func (c *OllamaClient) SetHost(host string) {
	c.host = host
}

func (c *OllamaClient) Model() string  { return c.model }
func (c *OllamaClient) Host() string   { return c.host }

func (c *OllamaClient) SetOption(key, raw string) error {
	// Try int, float, bool, string in order
	if i, err := strconv.Atoi(raw); err == nil {
		c.options[key] = i
		return nil
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		c.options[key] = f
		return nil
	}
	if b, err := strconv.ParseBool(raw); err == nil {
		c.options[key] = b
		return nil
	}
	c.options[key] = raw
	return nil
}

func (c *OllamaClient) AllOptions() map[string]interface{} {
	out := make(map[string]interface{}, len(c.options))
	for k, v := range c.options {
		out[k] = v
	}
	return out
}

type ollamaChatRequest struct {
	Model    string                 `json:"model"`
	Messages []ollamaMessage        `json:"messages"`
	Stream   bool                   `json:"stream"`
	Raw      bool                   `json:"raw,omitempty"`
	Think    *bool                  `json:"think,omitempty"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatResponse struct {
	Message ollamaMessage `json:"message"`
	Error   string        `json:"error,omitempty"`
}

type ollamaStreamChunk struct {
	Message      ollamaMessage `json:"message,omitempty"`
	Done         bool          `json:"done"`
	EvalCount    int           `json:"eval_count,omitempty"`
	EvalDuration int64         `json:"eval_duration,omitempty"`
	Error        string        `json:"error,omitempty"`
}

type ChatResult struct {
	Content   string
	EvalCount int
	TokPerSec float64
}

func (c *OllamaClient) prepareMessages(systemPrompt string, messages []Message) []ollamaMessage {
	if !c.raw {
		// Normal mode: let Ollama's template handle formatting
		ollamaMsgs := []ollamaMessage{}
		if systemPrompt != "" {
			ollamaMsgs = append(ollamaMsgs, ollamaMessage{Role: "system", Content: systemPrompt})
		}
		for _, m := range messages {
			ollamaMsgs = append(ollamaMsgs, ollamaMessage{Role: m.Role, Content: m.Content})
		}
		return ollamaMsgs
	}

	// Raw mode: format everything as ChatML so Ollama bypasses its template
	var sb strings.Builder
	if systemPrompt != "" {
		sb.WriteString("<|im_start|>system\n")
		sb.WriteString(systemPrompt)
		sb.WriteString("<|im_end|>\n")
	}
	for _, m := range messages {
		sb.WriteString("<|im_start|>")
		sb.WriteString(m.Role)
		sb.WriteString("\n")
		sb.WriteString(m.Content)
		sb.WriteString("<|im_end|>\n")
	}
	sb.WriteString("<|im_start|>assistant\n")

	return []ollamaMessage{{
		Role:    "user",
		Content: sb.String(),
	}}
}

func (c *OllamaClient) Chat(systemPrompt string, messages []Message) (string, error) {
	ollamaMsgs := c.prepareMessages(systemPrompt, messages)

	reqBody := ollamaChatRequest{
		Model:    c.model,
		Messages: ollamaMsgs,
		Stream:   false,
		Raw:      c.raw,
		Think:    c.think,
		Options:  copyMap(c.options),
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := c.host + "/api/chat"
	resp, err := c.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("POST %s: %w — is Ollama running?", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("ollama returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result ollamaChatResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w — body: %s", err, string(body))
	}

	if result.Error != "" {
		return "", fmt.Errorf("ollama error: %s", result.Error)
	}

	return result.Message.Content, nil
}

func (c *OllamaClient) ChatStream(systemPrompt string, messages []Message, onToken func(string)) (ChatResult, error) {
	ollamaMsgs := c.prepareMessages(systemPrompt, messages)

	reqBody := ollamaChatRequest{
		Model:    c.model,
		Messages: ollamaMsgs,
		Stream:   true,
		Raw:      c.raw,
		Think:    c.think,
		Options:  copyMap(c.options),
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return ChatResult{}, fmt.Errorf("marshal request: %w", err)
	}

	url := c.host + "/api/chat"
	resp, err := c.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return ChatResult{}, fmt.Errorf("POST %s: %w — is Ollama running?", url, err)
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return ChatResult{}, fmt.Errorf("ollama returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result ChatResult
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var chunk ollamaStreamChunk
		if err := json.Unmarshal(line, &chunk); err != nil {
			continue
		}

		if chunk.Error != "" {
			resp.Body.Close()
			return result, fmt.Errorf("ollama error: %s", chunk.Error)
		}

		if chunk.Message.Content != "" {
			result.Content += chunk.Message.Content
			if onToken != nil {
				onToken(chunk.Message.Content)
			}
		}

		if chunk.Done {
			result.EvalCount = chunk.EvalCount
			if chunk.EvalCount > 0 && chunk.EvalDuration > 0 {
				result.TokPerSec = float64(chunk.EvalCount) / (float64(chunk.EvalDuration) / 1e9)
			}
			break
		}
	}

	resp.Body.Close()
	return result, scanner.Err()
}

func copyMap(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}