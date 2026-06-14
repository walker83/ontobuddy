package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/walker/myonto/internal/config"
)

// TestAvailable 验证配置检查。
func TestAvailable(t *testing.T) {
	tests := []struct {
		name string
		c    *Client
		want bool
	}{
		{"全配置", &Client{BaseURL: "https://x", Model: "m"}, true},
		{"缺 model", &Client{BaseURL: "https://x"}, false},
		{"缺 baseURL", &Client{Model: "m"}, false},
		{"全空", &Client{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.Available(); got != tt.want {
				t.Errorf("Available = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFromConfigDefaults 验证：没传 provider/base_url 时不兜底（避免静默联外部）。
func TestFromConfigDefaults(t *testing.T) {
	// 确保 env 不影响。
	t.Setenv("MYONTO_LLM_BASE_URL", "")
	t.Setenv("MYONTO_LLM_API_KEY", "")
	t.Setenv("MYONTO_LLM_MODEL", "")

	c := FromConfig(config.LLM{})
	if c.BaseURL != "" {
		t.Errorf("无 provider 时 BaseURL 应为空，got %q", c.BaseURL)
	}
	if c.Model == "gpt-4o-mini" {
		t.Error("不应默认成 OpenAI 模型（避免静默联外部）")
	}
}

// TestChat_RequestFormat 用 mock server 验证请求体格式与响应解析。
func TestChat_RequestFormat(t *testing.T) {
	var capturedReq ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证 URL 路径、Authorization 头、Content-Type
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing Content-Type")
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing Authorization header")
		}
		_ = json.NewDecoder(r.Body).Decode(&capturedReq)

		// 返回 mock 响应
		resp := ChatResponse{
			ID:    "test-id",
			Model: capturedReq.Model,
		}
		resp.Choices = []struct {
			Index   int     `json:"index"`
			Message Message `json:"message"`
			Finish  string  `json:"finish_reason"`
		}{{
			Index:   0,
			Message: Message{Role: "assistant", Content: "hi back"},
			Finish:  "stop",
		}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := &Client{
		BaseURL: srv.URL,
		APIKey:  "test-key",
		Model:   "test-model",
		HTTP:    srv.Client(),
	}
	got, err := c.Chat(t.Context(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Chat 错误: %v", err)
	}
	if got != "hi back" {
		t.Errorf("got %q, want %q", got, "hi back")
	}
	if capturedReq.Model != "test-model" {
		t.Errorf("model = %q, want test-model", capturedReq.Model)
	}
	if len(capturedReq.Messages) != 1 || capturedReq.Messages[0].Content != "hi" {
		t.Errorf("messages 解析错误: %+v", capturedReq.Messages)
	}
}

// TestChat_NoAPIKey 验证无 key 时（Ollama 模式）仍可工作。
func TestChat_NoAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 不应带 Authorization 头
		if r.Header.Get("Authorization") != "" {
			t.Error("无 API key 时不应带 Authorization 头")
		}
		resp := ChatResponse{}
		resp.Choices = []struct {
			Index   int     `json:"index"`
			Message Message `json:"message"`
			Finish  string  `json:"finish_reason"`
		}{{
			Index:   0,
			Message: Message{Role: "assistant", Content: "ok"},
		}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Model: "m", HTTP: srv.Client()}
	got, err := c.Chat(t.Context(), []Message{{Role: "user", Content: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Errorf("got %q, want ok", got)
	}
}

// TestChat_HTTPError 验证 HTTP 错误响应被正确处理。
func TestChat_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limit", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, APIKey: "k", Model: "m", HTTP: srv.Client()}
	_, err := c.Chat(t.Context(), nil)
	if err == nil {
		t.Fatal("期望错误")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("错误信息应含状态码: %v", err)
	}
}

// TestChat_NotConfigured 验证未配置时清晰报错。
func TestChat_NotConfigured(t *testing.T) {
	c := &Client{}
	_, err := c.Chat(t.Context(), nil)
	if err == nil {
		t.Fatal("期望错误")
	}
	if !strings.Contains(err.Error(), "未配置") {
		t.Errorf("错误应含 '未配置': %v", err)
	}
}
