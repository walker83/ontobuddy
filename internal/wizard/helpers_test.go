package wizard

import (
	"os"

	"github.com/pelletier/go-toml/v2"

	"github.com/walker/myonto/internal/config"
	cryptox "github.com/walker/myonto/internal/crypto"
)

// readLLMRawForTest 直接读 .myonto.toml 解析 [llm] 段（不走透明解密）。
func readLLMRawForTest(path string) (config.LLM, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config.LLM{}, err
	}
	cfg := config.Config{}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return config.LLM{}, err
	}
	return cfg.LLM, nil
}

// decryptForTest 解密（测试用）。
func decryptForTest(token string) (string, error) {
	return cryptox.Decrypt(token)
}
