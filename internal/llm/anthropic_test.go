package llm

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/walker/myonto/internal/config"
)

// TestAnthropic_RequestFormat 验证 Anthropic 协议的请求头 + 请求体格式。
func TestAnthropic_RequestFormat(t *testing.T) {
	var gotPath, gotAPIKey, gotVersion, gotCT string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotCT = r.Header.Get("Content-Type")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		// 返回标准 Anthropic 响应
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_x","model":"claude","content":[{"type":"text","text":"hello back"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":3}}`))
	}))
	defer srv.Close()

	c := &Client{
		BaseURL:  srv.URL,
		APIKey:   "test-key-123",
		Model:    "claude-opus-4-5",
		Protocol: ProtocolAnthropic,
		HTTP:     srv.Client(),
	}
	got, err := c.Chat(t.Context(), []Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "hi"},
	})
	if err != nil {
		t.Fatalf("Chat 错误: %v", err)
	}
	if got != "hello back" {
		t.Errorf("got %q, want %q", got, "hello back")
	}
	if gotPath != "/v1/messages" {
		t.Errorf("path = %q, want /v1/messages", gotPath)
	}
	if gotAPIKey != "test-key-123" {
		t.Errorf("x-api-key = %q, want test-key-123", gotAPIKey)
	}
	if gotVersion != "2023-06-01" {
		t.Errorf("anthropic-version = %q, want 2023-06-01", gotVersion)
	}
	if !strings.HasPrefix(gotCT, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	// 关键：system 字段应在顶层，不在 messages 里
	if !strings.Contains(gotBody, `"system":"You are helpful."`) {
		t.Error("system 字段应在顶层")
	}
	if strings.Contains(gotBody, `"role":"system"`) {
		t.Error("messages 里不应有 role=system（Anthropic 协议）")
	}
	// max_tokens 应是必填且填了
	if !strings.Contains(gotBody, `"max_tokens":`) {
		t.Error("max_tokens 必填且应有值")
	}
}

// TestAnthropic_HTTPError 验证 Anthropic 错误响应被处理。
func TestAnthropic_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"invalid api key"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, APIKey: "bad", Model: "m", Protocol: ProtocolAnthropic, HTTP: srv.Client()}
	_, err := c.Chat(t.Context(), []Message{{Role: "user", Content: "x"}})
	if err == nil {
		t.Fatal("期望错误")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("应含状态码 401: %v", err)
	}
}

// TestAnthropic_NoAPIKey 验证没 key 时仍能发请求（Ollama 模式），但 Anthropic 协议必须有 key。
func TestAnthropic_NoAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "" {
			t.Error("无 key 时不应带 x-api-key 头")
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Model: "m", Protocol: ProtocolAnthropic, HTTP: srv.Client()}
	got, err := c.Chat(t.Context(), []Message{{Role: "user", Content: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Errorf("got %q, want ok", got)
	}
}

// TestFromConfig_ProviderLookup 验证从 provider 自动填 base_url/model/protocol。
func TestFromConfig_ProviderLookup(t *testing.T) {
	t.Setenv("MYONTO_LLM_BASE_URL", "")
	t.Setenv("MYONTO_LLM_API_KEY", "")
	t.Setenv("MYONTO_LLM_MODEL", "")

	c := FromConfig(config.LLM{Provider: "alibaba-coding"})
	if c.BaseURL != "https://coding.dashscope.aliyuncs.com/apps/anthropic" {
		t.Errorf("BaseURL = %q, want alibaba coding", c.BaseURL)
	}
	if c.Model != "qwen3.7-plus" {
		t.Errorf("Model = %q, want qwen3.7-plus", c.Model)
	}
	if c.Protocol != ProtocolAnthropic {
		t.Errorf("Protocol = %q, want anthropic", c.Protocol)
	}
}

// TestFromConfig_Override 验证用户在 config 里手动填的 base_url/model 覆盖 provider 默认。
func TestFromConfig_Override(t *testing.T) {
	c := FromConfig(config.LLM{
		Provider: "alibaba-coding",
		BaseURL:  "https://custom.example.com",
		Model:    "glm-5",
	})
	if c.BaseURL != "https://custom.example.com" {
		t.Errorf("BaseURL 应被覆盖: %q", c.BaseURL)
	}
	if c.Model != "glm-5" {
		t.Errorf("Model 应被覆盖: %q", c.Model)
	}
}
