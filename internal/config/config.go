// Package config 加载项目级配置文件 .myonto.toml。
//
// 配置示例：
//
//	base_iri = "http://example.org/"
//	data_file = "ontology.ttl"
//
//	[llm]
//	base_url = "https://api.deepseek.com/v1"
//	api_key = "sk-..."
//	model = "deepseek-chat"
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	cryptox "github.com/walker/myonto/internal/crypto"
)

// Config 是 .myonto.toml 的结构。
type Config struct {
	BaseIRI  string `toml:"base_iri"`  // 本项目本地命名空间的基础 IRI
	DataFile string `toml:"data_file"` // 本体数据文件名，默认 ontology.ttl
	Prefix   string `toml:"prefix"`    // 本地命名空间的前缀，默认 "ex"
	LLM      LLM    `toml:"llm"`       // LLM 配置（AI 命令使用）
}

// LLM 是云端 LLM 的配置。
//
// 推荐流程：用 myonto config llm set-key 写入加密的 api_key_token。
// 也可手动填明文 api_key（不推荐，会进 git 历史）。
type LLM struct {
	// Provider 是预设供应商 ID（如 "alibaba-coding"），决定默认 base_url/model/protocol。
	// 用户可单独覆盖下方的字段。
	Provider string `toml:"provider"`
	// BaseURL 不含路径，OpenAI 协议加 /chat/completions，Anthropic 协议加 /v1/messages。
	BaseURL string `toml:"base_url"`
	// APIKey 是明文 API key（不推荐，会进 git 历史）。优先用 APIKeyToken。
	APIKey string `toml:"api_key"`
	// APIKeyToken 是 Encrypt() 加密后的密文（base64）。运行时由 config.Load 透明解密到 APIKey。
	APIKeyToken string `toml:"api_key_token"`
	// Model 是模型 ID。
	Model string `toml:"model"`
	// Protocol "openai" / "anthropic"，默认 openai。可省略，provider 预设有。
	Protocol string `toml:"protocol"`
}

// Default 返回合理的默认配置。
func Default() Config {
	return Config{
		BaseIRI:  "http://example.org/",
		DataFile: "ontology.ttl",
		Prefix:   "ex",
	}
}

// ConfigFile 是项目级配置文件名。
const ConfigFile = ".myonto.toml"

// Find 从给定目录向上查找 .myonto.toml，返回找到的目录与配置。
// 找不到时返回 (dir, ErrNotFound)，dir 为起始查找目录。
func Find(startDir string) (string, Config, error) {
	dir := startDir
	for {
		path := filepath.Join(dir, ConfigFile)
		if _, err := os.Stat(path); err == nil {
			cfg, err := Load(path)
			if err != nil {
				return dir, Config{}, fmt.Errorf("load %s: %w", path, err)
			}
			return dir, cfg, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return startDir, Config{}, ErrNotFound
		}
		dir = parent
	}
}

// ErrNotFound 表示未找到配置文件。
var ErrNotFound = fmt.Errorf("myonto: %s not found (run 'myonto init' first)", ConfigFile)

// Load 从指定路径读取并解析配置文件。
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg := Default()
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	// 兜底默认值。
	if cfg.DataFile == "" {
		cfg.DataFile = "ontology.ttl"
	}
	if cfg.Prefix == "" {
		cfg.Prefix = "ex"
	}
	if cfg.BaseIRI == "" {
		cfg.BaseIRI = "http://example.org/"
	}
	// 解密 api_key_token（如果有）到 APIKey；清空 token 不让上层误用。
	if cfg.LLM.APIKeyToken != "" {
		decrypted, err := cryptox.Decrypt(cfg.LLM.APIKeyToken)
		if err != nil {
			return Config{}, fmt.Errorf("解密 api_key_token 失败（换机器后需重新 set-key）：%w", err)
		}
		cfg.LLM.APIKey = decrypted
		cfg.LLM.APIKeyToken = "" // 解密后丢弃明文 token
	}
	return cfg, nil
}

// Save 把配置写入指定路径。
//
// 文件权限 0o600（仅 owner 可读写）：因为该文件可能含加密 token，
// 或（兼容旧版）明文 api_key——任何一种都不应让同机其他用户读到。
func Save(path string, cfg Config) error {
	data, err := toml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
