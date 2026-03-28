package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/cpra/muon/message"
)

const (
	maxRetries     = 5
	retryBaseDelay = 1 * time.Second
)

type Config struct {
	BaseURL       string
	APIKey        string
	Model         string
	MaxTokens     int
	ContextLength int // 0 = auto-detect from model name
}

type Client struct {
	cfg          Config
	http         *http.Client
	mu           sync.Mutex
	modelInfo    *Model
	modelFetched bool
}

func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 5 * time.Minute},
	}
}

type chatRequest struct {
	Model     string                   `json:"model"`
	Messages  []message.Message        `json:"messages"`
	Tools     []map[string]interface{} `json:"tools,omitempty"`
	MaxTokens int                      `json:"max_tokens,omitempty"`
}

type chatChoice struct {
	Message message.Message `json:"message"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
	Usage   Usage        `json:"usage"`
}

type errorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Pricing holds per-token cost information returned by the provider's model
// listing endpoint. Values are USD per token as strings (e.g. "0.000003").
type Pricing struct {
	Prompt     string `json:"prompt,omitempty"`
	Completion string `json:"completion,omitempty"`
}

// CostInfo represents the calculated monetary cost for a single LLM turn.
type CostInfo struct {
	PromptCost     float64
	CompletionCost float64
	TotalCost      float64
}

func (c *Client) doRequest(ctx context.Context, method, path string, payload interface{}) ([]byte, error) {
	var bodyBytes []byte
	if payload != nil {
		var err error
		bodyBytes, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
	}

	for attempt := 0; ; attempt++ {
		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if c.cfg.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send request: %w", err)
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries {
			delay := retryBaseDelay * time.Duration(math.Pow(2, float64(attempt)))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		if resp.StatusCode != http.StatusOK {
			var errResp errorResponse
			if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
				return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, errResp.Error.Message)
			}
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, respBody)
		}

		return respBody, nil
	}
}

func (c *Client) Create(ctx context.Context, msgs []message.Message, tools []map[string]interface{}) (*message.Message, Usage, error) {
	body := chatRequest{
		Model:     c.cfg.Model,
		Messages:  msgs,
		Tools:     tools,
		MaxTokens: c.cfg.MaxTokens,
	}

	respBody, err := c.doRequest(ctx, http.MethodPost, "/chat/completions", body)
	if err != nil {
		return nil, Usage{}, err
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, Usage{}, fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, Usage{}, fmt.Errorf("no choices returned")
	}

	return &chatResp.Choices[0].Message, chatResp.Usage, nil
}

type Model struct {
	ID            string  `json:"id"`
	Object        string  `json:"object,omitempty"`
	Created       int64   `json:"created,omitempty"`
	OwnedBy       string  `json:"owned_by,omitempty"`
	ContextLength int     `json:"context_length,omitempty"`
	Pricing       Pricing `json:"pricing,omitempty"`
}

type listModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	respBody, err := c.doRequest(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}

	var result listModelsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}

	return result.Data, nil
}

// EnsureModelInfo fetches model metadata from the provider's /models endpoint
// and caches it for the lifetime of the client. Subsequent calls are no-ops.
// The fetch is best-effort: errors are returned but not cached so the caller
// can retry.
func (c *Client) EnsureModelInfo(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.modelFetched {
		return nil
	}

	models, err := c.ListModels(ctx)
	if err != nil {
		return err
	}

	c.modelFetched = true
	for i := range models {
		if models[i].ID == c.cfg.Model {
			info := models[i]
			c.modelInfo = &info
			break
		}
	}
	return nil
}

const defaultContextLength = 128000

// ContextLength returns the maximum context window size in tokens for the
// configured model. It uses (in order of precedence):
//  1. An explicitly configured ContextLength value
//  2. Metadata fetched from the provider's /models endpoint
//  3. A sensible default (128k)
func (c *Client) ContextLength() int {
	if c.cfg.ContextLength > 0 {
		return c.cfg.ContextLength
	}

	c.mu.Lock()
	info := c.modelInfo
	c.mu.Unlock()

	if info != nil && info.ContextLength > 0 {
		return info.ContextLength
	}

	return defaultContextLength
}

// CalculateCost returns the monetary cost for a given usage block based on
// the cached model pricing. Returns a zero-value CostInfo if pricing data is
// unavailable.
func (c *Client) CalculateCost(usage Usage) CostInfo {
	c.mu.Lock()
	info := c.modelInfo
	c.mu.Unlock()

	if info == nil {
		return CostInfo{}
	}

	promptPrice, _ := strconv.ParseFloat(info.Pricing.Prompt, 64)
	completionPrice, _ := strconv.ParseFloat(info.Pricing.Completion, 64)

	promptCost := float64(usage.PromptTokens) * promptPrice
	completionCost := float64(usage.CompletionTokens) * completionPrice

	return CostInfo{
		PromptCost:     promptCost,
		CompletionCost: completionCost,
		TotalCost:      promptCost + completionCost,
	}
}
