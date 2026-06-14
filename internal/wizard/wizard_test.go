package wizard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/walker/myonto/internal/config"
	"github.com/walker/myonto/internal/llm"
)

// TestApply_Plain 验证 Apply 写入基础设置到 .myonto.toml。
func TestApply_Plain(t *testing.T) {
	dir := t.TempDir()
	res := &Result{
		BaseIRI:  "http://mytest.org/",
		Prefix:   "mt",
		DataFile: "my.ttl",
		SetupLLM: false,
	}
	if err := Apply(dir, res); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, config.ConfigFile)
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatal("配置文件未创建")
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseIRI != "http://mytest.org/" {
		t.Errorf("BaseIRI = %q, want http://mytest.org/", cfg.BaseIRI)
	}
	if cfg.Prefix != "mt" {
		t.Errorf("Prefix = %q, want mt", cfg.Prefix)
	}
	if cfg.DataFile != "my.ttl" {
		t.Errorf("DataFile = %q, want my.ttl", cfg.DataFile)
	}
}

// TestApply_WithLLM 验证 Apply 加密 LLM key 写入。
func TestApply_WithLLM(t *testing.T) {
	dir := t.TempDir()
	res := &Result{
		BaseIRI:   "http://x.org/",
		Prefix:    "x",
		DataFile:  "ontology.ttl",
		SetupLLM:  true,
		Provider:  "alibaba-coding",
		Model:     "", // 用 provider 默认
		APIKeyRaw: "sk-test-secret-key",
	}
	if err := Apply(dir, res); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, config.ConfigFile)
	// 不走 config.Load（会触发透明解密），直接读源文件看密文
	llmRaw, err := readLLMRawForTest(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if llmRaw.Provider != "alibaba-coding" {
		t.Errorf("Provider = %q, want alibaba-coding", llmRaw.Provider)
	}
	if llmRaw.APIKeyToken == "" {
		t.Error("APIKeyToken 应有密文")
	}
	if llmRaw.APIKey != "" {
		t.Errorf("APIKey 应留空（明文不落盘），got %q", llmRaw.APIKey)
	}
	if llmRaw.Model == "" {
		t.Error("Model 应填 provider 默认")
	}
	// 解密后应得到原明文
	decrypted, err := decryptForTest(llmRaw.APIKeyToken)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if decrypted != "sk-test-secret-key" {
		t.Errorf("解密值 = %q, want sk-test-secret-key", decrypted)
	}
}

// TestApply_FilePermission 验证 Apply 写出的 .myonto.toml 权限是 0600。
func TestApply_FilePermission(t *testing.T) {
	dir := t.TempDir()
	res := &Result{
		BaseIRI:   "http://x.org/",
		Prefix:    "x",
		DataFile:  "ontology.ttl",
		SetupLLM:  true,
		Provider:  "openai",
		APIKeyRaw: "sk-test",
	}
	if err := Apply(dir, res); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, config.ConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("文件权限 = %o, want 0600", perm)
	}
}

// TestErrNoTTY 验证 ErrNoTTY 是已定义的哨兵错误。
func TestErrNoTTY(t *testing.T) {
	if ErrNoTTY == nil {
		t.Error("ErrNoTTY 不应为 nil")
	}
	if !strings.Contains(ErrNoTTY.Error(), "TTY") {
		t.Errorf("错误信息应含 TTY: %v", ErrNoTTY)
	}
}

// TestBuiltinProviders_RequiredFields 验证关键 provider 字段完整。
func TestBuiltinProviders_RequiredFields(t *testing.T) {
	if _, ok := llm.BuiltinProviders["alibaba-coding"]; !ok {
		t.Fatal("缺少默认 provider alibaba-coding")
	}
	p := llm.BuiltinProviders["alibaba-coding"]
	if p.Protocol != "anthropic" {
		t.Errorf("alibaba-coding 应是 anthropic 协议, got %q", p.Protocol)
	}
	if p.BaseURL == "" {
		t.Error("BaseURL 不应为空")
	}
	if p.DefaultModel == "" {
		t.Error("DefaultModel 不应为空")
	}
	if len(p.Models) == 0 {
		t.Error("Models 应至少有一个")
	}
}
