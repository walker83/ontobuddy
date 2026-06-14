package llm

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAnthropic_MultipleSystemMessagesConcatenated 验证多条 system 消息被拼接，
// 而不是静默只保留最后一条（之前的 bug）。
func TestAnthropic_MultipleSystemMessagesConcatenated(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
	}))
	defer srv.Close()

	c := &Client{
		BaseURL:  srv.URL,
		APIKey:   "k",
		Model:    "m",
		Protocol: ProtocolAnthropic,
		HTTP:     srv.Client(),
	}
	_, err := c.Chat(t.Context(), []Message{
		{Role: "system", Content: "You are a poet."},
		{Role: "system", Content: "Always answer in haiku."},
		{Role: "user", Content: "hi"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 两条 system 都应在顶层 system 字段里出现（拼接）
	if !strings.Contains(gotBody, "You are a poet.") {
		t.Error("第一条 system 消息丢失")
	}
	if !strings.Contains(gotBody, "Always answer in haiku.") {
		t.Error("第二条 system 消息丢失")
	}
	if strings.Contains(gotBody, `"role":"system"`) {
		t.Error("messages 里不应有 role=system")
	}
}

// TestTruncateBody_LongBody 验证错误信息中的 body 被截断。
func TestTruncateBody_LongBody(t *testing.T) {
	long := strings.Repeat("x", 2000)
	got := truncateBody([]byte(long))
	if len(got) > 600 { // 512 + "...(截断)"
		t.Errorf("截断后长度 %d, 应 ≤ ~530", len(got))
	}
	if !strings.Contains(got, "截断") {
		t.Error("截断后应有提示")
	}
}

// TestTruncateBody_ShortBody 验证短 body 原样返回。
func TestTruncateBody_ShortBody(t *testing.T) {
	short := "short error"
	got := truncateBody([]byte(short))
	if got != short {
		t.Errorf("短 body 应原样返回: got %q", got)
	}
}

// TestErrorBody_TruncatedInErrorResponse 验证实际错误路径里 body 也被截断。
func TestErrorBody_TruncatedInErrorResponse(t *testing.T) {
	// 构造一个总长 > 512 字节的 body，并在前 512 字节之外放一个独特标记。
	prefix := strings.Repeat("x", 512)            // 占满截断窗口
	marker := "UNIQUE_TAIL_MARKER_OUTSIDE_WINDOW" // 应被截断掉
	fullBody := prefix + marker                   // 共 ~556 字节
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, fullBody, http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, APIKey: "k", Model: "m", HTTP: srv.Client()}
	_, err := c.Chat(t.Context(), []Message{{Role: "user", Content: "x"}})
	if err == nil {
		t.Fatal("期望错误")
	}
	errMsg := err.Error()
	// 截断窗口外的 marker 不应出现在错误里
	if strings.Contains(errMsg, marker) {
		t.Error("错误信息不应包含截断窗口之外的 body 内容（截断失效）")
	}
	if !strings.Contains(errMsg, "截断") {
		t.Error("长 body 错误应含截断提示")
	}
}
