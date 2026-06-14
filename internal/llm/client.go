// Package llm 提供 LLM 客户端，支持两种协议：
//   - OpenAI Chat Completions（最常见）
//   - Anthropic Messages（阿里云 DashScope 的 Anthropic 兼容端点用这个）
//
// 零第三方依赖，纯 net/http 实现。配置从 .myonto.toml 的 [llm] 节或
// 环境变量 MYONTO_LLM_* 读取。
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/walker/myonto/internal/config"
)

// Message 是 chat-completions 的一条消息（两种协议都复用这个结构）。
type Message struct {
	Role    string `json:"role"`    // "system" / "user" / "assistant"
	Content string `json:"content"` // 文本内容
}

// ChatRequest 是 OpenAI 兼容 chat-completions 的请求体。
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// ChatResponse 是 OpenAI 兼容 chat-completions 的响应。
type ChatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int     `json:"index"`
		Message Message `json:"message"`
		Finish  string  `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// AnthropicRequest 是 Anthropic Messages API 的请求体。
// 注意：system 不在 messages 里，是顶层字段；max_tokens 是必填。
type AnthropicRequest struct {
	Model       string    `json:"model"`
	System      string    `json:"system,omitempty"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature,omitempty"`
}

// AnthropicResponse 是 Anthropic Messages API 的响应。
type AnthropicResponse struct {
	ID    string `json:"id"`
	Model string `json:"model"`
	// 兼容两种常见路径：根级 content 和 choices[].message.content
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// Protocol 是 LLM 通信协议类型。
type Protocol string

const (
	ProtocolOpenAI    Protocol = "openai"
	ProtocolAnthropic Protocol = "anthropic"
)

// Client 是 LLM HTTP 客户端。
type Client struct {
	BaseURL  string
	APIKey   string
	Model    string
	Protocol Protocol // "openai" / "anthropic"
	HTTP     *http.Client
}

// FromConfig 从 config 构造 Client，缺省值由环境变量兜底。
func FromConfig(c config.LLM) *Client {
	proto := Protocol(c.Protocol)
	if proto == "" {
		proto = ProtocolOpenAI
	}
	base := c.BaseURL
	if base == "" {
		base = os.Getenv("MYONTO_LLM_BASE_URL")
	}
	key := c.APIKey
	if key == "" {
		key = os.Getenv("MYONTO_LLM_API_KEY")
	}
	model := c.Model
	if model == "" {
		model = os.Getenv("MYONTO_LLM_MODEL")
	}
	// 如果用户没填 base_url 但填了 provider，尝试用 provider 的默认值。
	if base == "" && c.Provider != "" {
		if p := GetProvider(c.Provider); p != nil {
			base = p.BaseURL
			if model == "" {
				model = p.DefaultModel
			}
			if proto == ProtocolOpenAI && p.Protocol == "anthropic" {
				proto = ProtocolAnthropic
			}
		}
	}
	return &Client{
		BaseURL:  base,
		APIKey:   key,
		Model:    model,
		Protocol: proto,
		HTTP:     &http.Client{Timeout: 120 * time.Second},
	}
}

// Available 判断客户端是否至少能发起请求。
func (c *Client) Available() bool {
	return c.BaseURL != "" && c.Model != ""
}

// Chat 发送一次非流式 chat 请求，根据 Protocol 分发到对应实现。
func (c *Client) Chat(ctx context.Context, messages []Message) (string, error) {
	if !c.Available() {
		return "", fmt.Errorf("llm 未配置：在 .myonto.toml 的 [llm] 节或环境变量 MYONTO_LLM_* 设置 base_url/api_key/model")
	}
	switch c.Protocol {
	case ProtocolAnthropic:
		return c.chatAnthropic(ctx, messages)
	default:
		return c.chatOpenAI(ctx, messages)
	}
}

// chatOpenAI 走 OpenAI Chat Completions 协议。
func (c *Client) chatOpenAI(ctx context.Context, messages []Message) (string, error) {
	body, err := json.Marshal(ChatRequest{Model: c.Model, Messages: messages})
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm 请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm 返回 %d: %s", resp.StatusCode, truncateBody(data))
	}
	var parsed ChatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("llm 响应解析失败: %w (body: %s)", err, truncateBody(data))
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("llm 响应无 choices: %s", truncateBody(data))
	}
	return parsed.Choices[0].Message.Content, nil
}

// chatAnthropic 走 Anthropic Messages 协议。
//
// 请求/响应要点（与 OpenAI 关键差异）：
//   - 端点: POST {BaseURL}/v1/messages
//   - 认证: x-api-key: <KEY> + anthropic-version: 2023-06-01
//   - system 字段在 messages 外，是顶层 string
//   - max_tokens 必填（默认 1024）
//   - messages 里 role 只接受 user / assistant
func (c *Client) chatAnthropic(ctx context.Context, messages []Message) (string, error) {
	// 把 system 抽出来作为顶层字段；user/assistant 进入 messages。
	// Anthropic 只允许顶层一个 system 字符串，所以多条 system 消息会拼接
	// （而非静默覆盖——避免内容丢失）。多条之间用换行分隔。
	var systemParts []string
	filtered := make([]Message, 0, len(messages))
	for _, m := range messages {
		if m.Role == "system" {
			if m.Content != "" {
				systemParts = append(systemParts, m.Content)
			}
			continue
		}
		filtered = append(filtered, m)
	}
	systemText := strings.Join(systemParts, "\n\n")
	// Anthropic Messages API 要求 messages 至少含 1 条 user/assistant；
	// 若调用方只传了 system（健壮性边界），补一条空 user 避免被服务端 4xx 拒绝。
	if len(filtered) == 0 {
		filtered = []Message{{Role: "user", Content: "(无内容)"}}
	}
	maxTokens := 1024
	body, err := json.Marshal(AnthropicRequest{
		Model:     c.Model,
		System:    systemText,
		Messages:  filtered,
		MaxTokens: maxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("x-api-key", c.APIKey)
	}
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm 请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm 返回 %d: %s", resp.StatusCode, truncateBody(data))
	}
	var parsed AnthropicResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("llm 响应解析失败: %w (body: %s)", err, truncateBody(data))
	}
	// 优先从 content[] 取（标准 Anthropic 路径）
	if len(parsed.Content) > 0 {
		return parsed.Content[0].Text, nil
	}
	// 退化：部分兼容实现用 choices[].message.content
	if len(parsed.Choices) > 0 {
		return parsed.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("llm 响应无 content: %s", truncateBody(data))
}

// truncateBody 把响应体截断到 512 字节用于错误信息。
// 防止服务端在错误响应里返回大段敏感/调试信息时全部回显给用户。
func truncateBody(data []byte) string {
	const max = 512
	if len(data) <= max {
		return string(data)
	}
	return string(data[:max]) + "...(截断)"
}
