package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OllamaClient struct {
	host   string
	model  string
	client *http.Client
}

func NewOllamaClient(host, model string) *OllamaClient {
	return &OllamaClient{
		host:  host,
		model: model,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

type ollamaChatRequest struct {
	Model    string            `json:"model"`
	Messages []ollamaMessage   `json:"messages"`
	Stream   bool              `json:"stream"`
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

func (c *OllamaClient) Chat(systemPrompt string, messages []Message) (string, error) {
	ollamaMsgs := []ollamaMessage{}

	if systemPrompt != "" {
		ollamaMsgs = append(ollamaMsgs, ollamaMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	for _, m := range messages {
		ollamaMsgs = append(ollamaMsgs, ollamaMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	reqBody := ollamaChatRequest{
		Model:    c.model,
		Messages: ollamaMsgs,
		Stream:   false,
		Options: map[string]interface{}{
			"temperature": 0.7,
			"num_predict": 1024,
			"num_ctx":     4096,
		},
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
	ollamaMsgs := []ollamaMessage{}

	if systemPrompt != "" {
		ollamaMsgs = append(ollamaMsgs, ollamaMessage{Role: "system", Content: systemPrompt})
	}
	for _, m := range messages {
		ollamaMsgs = append(ollamaMsgs, ollamaMessage{Role: m.Role, Content: m.Content})
	}

	reqBody := ollamaChatRequest{
		Model:    c.model,
		Messages: ollamaMsgs,
		Stream:   true,
		Options: map[string]interface{}{
			"temperature": 0.7,
			"num_predict": 1024,
			"num_ctx":     4096,
		},
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