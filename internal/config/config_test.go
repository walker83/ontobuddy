package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cryptox "github.com/walker/myonto/internal/crypto"
)

// TestLoad_DecryptsToken 验证 Load 时 api_key_token 透明解密到 api_key。
func TestLoad_DecryptsToken(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ConfigFile)

	// 先加密一个 token
	plain := "sk-test-secret-12345"
	token, err := cryptox.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}

	// 写配置文件（含 token 字段）
	toml := `base_iri = "http://example.org/"
data_file = "ontology.ttl"
prefix = "ex"

[llm]
provider = "alibaba-coding"
base_url = "https://coding.dashscope.aliyuncs.com/apps/anthropic"
api_key_token = "` + token + `"
model = "qwen3.7-plus"
protocol = "anthropic"
`
	if err := os.WriteFile(cfgPath, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.APIKey != plain {
		t.Errorf("APIKey = %q, want %q", cfg.LLM.APIKey, plain)
	}
	if cfg.LLM.APIKeyToken != "" {
		t.Error("APIKeyToken 透明解密后应被清空")
	}
}

// TestLoad_BadToken 验证无效 token 报错（不是 panic / 静默返回空）。
func TestLoad_BadToken(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ConfigFile)
	toml := `[llm]
api_key_token = "!!!not base64!!!"
`
	if err := os.WriteFile(cfgPath, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("期望错误")
	}
	if !strings.Contains(err.Error(), "api_key_token") {
		t.Errorf("错误应提到 api_key_token: %v", err)
	}
}

// TestLoad_PlainAPIKeyWorks 验证旧版明文 api_key 仍能工作（兼容）。
func TestLoad_PlainAPIKeyWorks(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ConfigFile)
	toml := `[llm]
api_key = "sk-plain-12345"
`
	if err := os.WriteFile(cfgPath, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.APIKey != "sk-plain-12345" {
		t.Errorf("明文 api_key 应被保留: got %q", cfg.LLM.APIKey)
	}
}

// TestSave_FilePermission0600 验证 Save 写出的文件权限是 0600，
// 而非 0644（含明文 key 兼容路径或加密 token 时，权限应仅限 owner）。
func TestSave_FilePermission0600(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ConfigFile)
	cfg := Default()
	cfg.LLM.APIKey = "sensitive-key"

	if err := Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("文件权限 = %o, want 0600", perm)
	}
}
