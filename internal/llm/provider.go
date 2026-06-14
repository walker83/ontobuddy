package llm

// Provider 表示一个 LLM 服务供应商的预设。
type Provider struct {
	// ID 用于 .myonto.toml 的 provider = "..." 配置项
	ID string
	// Name 人类可读名
	Name string
	// Protocol 是 "openai" 或 "anthropic"
	Protocol string
	// BaseURL 不含路径；调用时按协议加 /v1/chat/completions 或 /v1/messages
	BaseURL string
	// Models 推荐模型列表
	Models []string
	// DefaultModel 默认模型（推荐首选）
	DefaultModel string
	// APIStyle "openai-headers" / "anthropic-headers"；默认随 Protocol
	APIStyle string
}

// BuiltinProviders 是内置的供应商预设。
// 用户可通过 .myonto.toml 的 [llm] 自由覆盖 base_url/model。
var BuiltinProviders = map[string]Provider{
	"alibaba-coding": {
		ID:       "alibaba-coding",
		Name:     "阿里云编程套餐 (DashScope Anthropic 兼容)",
		Protocol: "anthropic",
		BaseURL:  "https://coding.dashscope.aliyuncs.com/apps/anthropic",
		Models: []string{
			"qwen3.7-plus", // 通义千问 3.7
			"glm-5",        // 智谱 GLM-5
		},
		DefaultModel: "qwen3.7-plus",
	},
	"openai": {
		ID:           "openai",
		Name:         "OpenAI 官方",
		Protocol:     "openai",
		BaseURL:      "https://api.openai.com/v1",
		Models:       []string{"gpt-4o", "gpt-4o-mini", "gpt-4.1", "gpt-4.1-mini", "o1", "o1-mini"},
		DefaultModel: "gpt-4o-mini",
	},
	"deepseek": {
		ID:           "deepseek",
		Name:         "DeepSeek",
		Protocol:     "openai",
		BaseURL:      "https://api.deepseek.com/v1",
		Models:       []string{"deepseek-chat", "deepseek-reasoner"},
		DefaultModel: "deepseek-chat",
	},
	"ollama": {
		ID:           "ollama",
		Name:         "Ollama（本地）",
		Protocol:     "openai",
		BaseURL:      "http://localhost:11434/v1",
		Models:       []string{"qwen2.5", "llama3.1", "mistral", "gemma2"},
		DefaultModel: "qwen2.5",
	},
}

// GetProvider 按 ID 取供应商，未知返回 nil。
func GetProvider(id string) *Provider {
	p, ok := BuiltinProviders[id]
	if !ok {
		return nil
	}
	return &p
}
