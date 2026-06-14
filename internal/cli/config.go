package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/pelletier/go-toml/v2"

	"github.com/walker/myonto/internal/config"
	cryptox "github.com/walker/myonto/internal/crypto"
	"github.com/walker/myonto/internal/llm"
	"github.com/walker/myonto/internal/wizard"
)

// ConfigCmd 是 myonto config 的命令组。
type ConfigCmd struct {
	LLM   ConfigLLMCmd   `cmd:"" help:"管理 LLM 配置（provider / api_key / base_url）"`
	Setup ConfigSetupCmd `cmd:"" help:"重跑 TUI 向导（修改 base_iri / prefix / LLM 等所有设置）"`
}

// ConfigLLMCmd 是 config llm 的子命令组。
type ConfigLLMCmd struct {
	SetKey        ConfigLLMSetKeyCmd        `cmd:"" help:"交互式设置并加密保存 API key"`
	Show          ConfigLLMShowCmd          `cmd:"" help:"显示当前 LLM 配置（key 隐藏）"`
	Test          ConfigLLMTestCmd          `cmd:"" help:"发送测试请求验证连通性"`
	ListProviders ConfigLLMListProvidersCmd `cmd:"" help:"列出所有内置供应商"`
}

// --- config llm set-key ---

// ConfigLLMSetKeyCmd 交互式设置 API key：提示输入明文、加密后写入配置文件。
type ConfigLLMSetKeyCmd struct {
	Provider string `help:"供应商 ID（如 alibaba-coding / openai），省略则用配置文件已有" placeholder:"PROVIDER"`
	Model    string `help:"指定模型（省略则用 provider 默认）" placeholder:"MODEL"`
	Key      string `help:"API key 明文（传参用，慎用；不传则交互式提示输入）" placeholder:"KEY"`
}

// Run 交互式设置。
func (c *ConfigLLMSetKeyCmd) Run() error {
	dir, cfg, err := loadConfigInCwd()
	if err != nil {
		return err
	}

	// 决定 provider：CLI 参数 > 已有配置 > 默认提示
	if c.Provider != "" {
		cfg.LLM.Provider = c.Provider
	}
	if cfg.LLM.Provider == "" {
		// 询问用户选 provider
		p, err := promptChooseProvider()
		if err != nil {
			return err
		}
		cfg.LLM.Provider = p
	}

	// 验证 provider 合法
	prov := llm.GetProvider(cfg.LLM.Provider)
	if prov == nil {
		return fmt.Errorf("未知 provider %q（用 myonto config llm list-providers 查看）", cfg.LLM.Provider)
	}

	// 填默认 base_url / model（用户可后续覆盖）
	if cfg.LLM.BaseURL == "" {
		cfg.LLM.BaseURL = prov.BaseURL
	}
	if c.Model != "" {
		cfg.LLM.Model = c.Model
	} else if cfg.LLM.Model == "" {
		cfg.LLM.Model = prov.DefaultModel
	}
	if cfg.LLM.Protocol == "" {
		cfg.LLM.Protocol = prov.Protocol
	}

	// 读 key：CLI > 提示
	var key string
	if c.Key != "" {
		fmt.Fprintln(os.Stderr, "⚠️  --key 在命令行传入会被 shell 历史记录，请用交互模式")
		key = c.Key
	} else {
		// 交互式：隐藏输入
		fmt.Fprintf(os.Stderr, "输入 %s (%s) 的 API key（输入隐藏，回车确认）：\n> ", prov.Name, cfg.LLM.Provider)
		raw, err := readPassword()
		if err != nil {
			return fmt.Errorf("读 key: %w", err)
		}
		key = strings.TrimSpace(string(raw))
		if key == "" {
			return fmt.Errorf("未输入 key")
		}
	}

	// 加密
	token, err := cryptox.Encrypt(key)
	if err != nil {
		return fmt.Errorf("加密失败: %w", err)
	}

	// 清空明文 api_key，写入 token
	cfg.LLM.APIKey = "" // 显式清空
	cfg.LLM.APIKeyToken = token

	// 保存
	cfgPath := filepath.Join(dir, config.ConfigFile)
	if err := config.Save(cfgPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "✓ API key 已加密保存到 %s\n", cfgPath)
	fmt.Fprintf(os.Stdout, "  provider:  %s\n", cfg.LLM.Provider)
	fmt.Fprintf(os.Stdout, "  base_url:  %s\n", cfg.LLM.BaseURL)
	fmt.Fprintf(os.Stdout, "  model:     %s\n", cfg.LLM.Model)
	fmt.Fprintf(os.Stdout, "  protocol:  %s\n", cfg.LLM.Protocol)
	fmt.Fprintf(os.Stdout, "  机器指纹:   %s（换机器后无法解密，需重新 set-key）\n", cryptox.StableFingerprintHash())
	return nil
}

// --- config llm show ---

// ConfigLLMShowCmd 显示配置。
type ConfigLLMShowCmd struct{}

// Run 显示。
//
// 同时呈现「磁盘原始字段」与「运行时生效值」：前者直接读 .myonto.toml，
// 后者经过 llm.FromConfig（含 provider 预设合并 / 环境变量兜底）。
// 这样只写了 `provider = "alibaba-coding"` 的用户也能看到实际会用的 base_url。
func (c *ConfigLLMShowCmd) Run() error {
	dir, _, err := loadConfigInCwd()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(dir, config.ConfigFile)
	raw, err := readLLMRaw(cfgPath)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "provider:    %s\n", emptyAs(raw.Provider, "(未设置)"))
	fmt.Fprintf(os.Stdout, "base_url:    %s\n", emptyAs(raw.BaseURL, "(未设置)"))
	fmt.Fprintf(os.Stdout, "model:       %s\n", emptyAs(raw.Model, "(未设置)"))
	fmt.Fprintf(os.Stdout, "protocol:    %s\n", emptyAs(raw.Protocol, "(未设置)"))
	// key
	if raw.APIKeyToken != "" {
		fmt.Fprintf(os.Stdout, "api_key:     (已加密存储，token 长度 %d)\n", len(raw.APIKeyToken))
		fmt.Fprintf(os.Stdout, "  机器指纹:  %s\n", cryptox.StableFingerprintHash())
	} else if raw.APIKey != "" {
		// 兼容旧版明文
		masked := maskKey(raw.APIKey)
		fmt.Fprintf(os.Stdout, "api_key:     %s  ⚠️ 明文存储（建议用 set-key 加密）\n", masked)
	} else {
		fmt.Fprintln(os.Stdout, "api_key:     (未设置)")
	}

	// 运行时生效值（含 provider 预设 + 环境变量兜底）。若与磁盘字段一致就不重复显示。
	effective := llm.FromConfig(raw)
	fmt.Fprintln(os.Stdout, "\n运行时生效值（含 provider 预设 / 环境变量）：")
	fmt.Fprintf(os.Stdout, "  base_url:  %s%s\n", effective.BaseURL, sourceTag(raw.BaseURL, effective.BaseURL, raw.Provider))
	fmt.Fprintf(os.Stdout, "  model:     %s%s\n", effective.Model, sourceTag(raw.Model, effective.Model, raw.Provider))
	fmt.Fprintf(os.Stdout, "  protocol:  %s\n", string(effective.Protocol))
	if !effective.Available() {
		fmt.Fprintln(os.Stdout, "  ⚠️ 仍不可用（缺 base_url 或 model）")
	}
	return nil
}

// sourceTag 标注某生效值相对磁盘字段是否被预设/环境变量补齐。
// 若 raw 非空但与 effective 相等，说明用磁盘值；若 raw 为空而 effective 非空，
// 说明来自 provider 预设或环境变量。
func sourceTag(raw, effective, provider string) string {
	if raw != "" {
		return "  (磁盘)"
	}
	if effective == "" {
		return ""
	}
	if provider != "" {
		return "  (provider 预设)"
	}
	return "  (环境变量)"
}

// --- config llm test ---

// ConfigLLMTestCmd 测连通性。
type ConfigLLMTestCmd struct{}

// Run 测连通。
func (c *ConfigLLMTestCmd) Run() error {
	_, cfg, err := loadConfigInCwd()
	if err != nil {
		return err
	}
	c2 := llm.FromConfig(cfg.LLM)
	if !c2.Available() {
		return fmt.Errorf("未配置：先用 myonto config llm set-key")
	}
	fmt.Fprintf(os.Stderr, "→ POST %s\n", testURL(c2))
	fmt.Fprintf(os.Stderr, "  model: %s\n", c2.Model)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := c2.Chat(ctx, []llm.Message{{Role: "user", Content: "ping"}})
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗ 失败")
		return err
	}
	fmt.Fprintln(os.Stderr, "✓ 成功")
	fmt.Fprintf(os.Stdout, "LLM 回应：%s\n", truncate(resp, 200))
	return nil
}

// testURL 返回 c 会打的端点 URL（仅展示）。
func testURL(c *llm.Client) string {
	if c.Protocol == llm.ProtocolAnthropic {
		return c.BaseURL + "/v1/messages"
	}
	return c.BaseURL + "/chat/completions"
}

// --- config llm list-providers ---

// ConfigLLMListProvidersCmd 列出内置供应商。
type ConfigLLMListProvidersCmd struct{}

// Run 列出。
func (c *ConfigLLMListProvidersCmd) Run() error {
	fmt.Fprintln(os.Stdout, "内置供应商：")
	for id, p := range llm.BuiltinProviders {
		fmt.Fprintf(os.Stdout, "\n  %s\n", id)
		fmt.Fprintf(os.Stdout, "    名称:     %s\n", p.Name)
		fmt.Fprintf(os.Stdout, "    协议:     %s\n", p.Protocol)
		fmt.Fprintf(os.Stdout, "    base_url: %s\n", p.BaseURL)
		fmt.Fprintf(os.Stdout, "    默认模型: %s\n", p.DefaultModel)
		fmt.Fprintf(os.Stdout, "    可用模型: %s\n", strings.Join(p.Models, ", "))
	}
	fmt.Fprintln(os.Stdout, "\n设置：myonto config llm set-key <provider>")
	return nil
}

// --- 共用 ---

// loadConfigInCwd 加载当前目录的 .myonto.toml。
func loadConfigInCwd() (string, config.Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", config.Config{}, err
	}
	dir, cfg, err := config.Find(cwd)
	if err != nil {
		return "", config.Config{}, err
	}
	return dir, cfg, nil
}

// promptChooseProvider 交互式选 provider。
func promptChooseProvider() (string, error) {
	ids := make([]string, 0, len(llm.BuiltinProviders))
	for id := range llm.BuiltinProviders {
		ids = append(ids, id)
	}
	// 稳定顺序
	sortStrings(ids)
	fmt.Fprintln(os.Stderr, "请选择供应商（输入 ID 回车）：")
	for i, id := range ids {
		p := llm.BuiltinProviders[id]
		fmt.Fprintf(os.Stderr, "  %d) %s  -  %s\n", i+1, id, p.Name)
	}
	fmt.Fprint(os.Stderr, "> ")
	var line string
	if _, err := fmt.Fscan(os.Stdin, &line); err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	for _, id := range ids {
		if id == line {
			return id, nil
		}
	}
	// 允许数字选
	if n, err := parseInt(line); err == nil {
		if n >= 1 && n <= len(ids) {
			return ids[n-1], nil
		}
	}
	return "", fmt.Errorf("无效选择: %q", line)
}

// readPassword 隐藏输入读一行。
func readPassword() ([]byte, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		// 进入隐藏模式
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			// 退回到非隐藏模式
			return readLineNoEcho()
		}
		defer term.Restore(fd, oldState)

		var buf []byte
		buf = append(buf, readUntilNewline(fd)...)
		// 换行
		fmt.Fprintln(os.Stderr)
		return buf, nil
	}
	// 非 TTY：直接读一行
	return readLineNoEcho()
}

// readUntilNewline 读一个字符直到 \r 或 \n。
func readUntilNewline(fd int) []byte {
	var buf []byte
	b := make([]byte, 1)
	for {
		n, err := syscall.Read(fd, b)
		if err != nil || n == 0 {
			break
		}
		if b[0] == '\r' || b[0] == '\n' {
			break
		}
		// 退格
		if b[0] == 127 || b[0] == 8 {
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				fmt.Fprint(os.Stderr, "\b \b")
			}
			continue
		}
		// Ctrl+C
		if b[0] == 3 {
			syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		}
		buf = append(buf, b[0])
		fmt.Fprint(os.Stderr, "*")
	}
	return buf
}

// readLineNoEcho 非 TTY 时的 fallback（bufio.Scanner）。
func readLineNoEcho() ([]byte, error) {
	var line strings.Builder
	buf := make([]byte, 1024)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			break
		}
		line.Write(buf[:n])
		if buf[n-1] == '\n' {
			break
		}
	}
	return []byte(strings.TrimRight(line.String(), "\r\n")), nil
}

// sortStrings 简单字典序排序（不引 sort 包避免新依赖）。
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// ConfigSetupCmd 实现 config setup：重跑 TUI 向导覆盖当前所有设置。
type ConfigSetupCmd struct{}

// Run 重走向导。
//
// 与 init 的区别：必须先有 .myonto.toml；向导预填现有值，用户可以改任何字段。
func (c *ConfigSetupCmd) Run() error {
	dir, cfg, err := loadConfigInCwd()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(dir, config.ConfigFile)

	// 探测示例本体
	examplesPath := findExamples()

	res, err := wizard.Run(os.Stdout, os.Stderr, &cfg, examplesPath)
	if err != nil {
		if err == wizard.ErrNoTTY {
			return fmt.Errorf("无 TTY 环境，请用 myonto config llm set-key 等命令手动配置")
		}
		if err == wizard.ErrUserCancelled {
			fmt.Fprintln(os.Stdout, "已取消，配置未修改。")
			return nil
		}
		return err
	}

	// 写入
	if err := wizard.Apply(dir, res); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "✓ 配置已更新：%s\n", cfgPath)
	return nil
}

// readLLMRaw 直接读 .myonto.toml 文件，仅解析 [llm] 段。
// 不走 config.Load，所以不会触发透明解密——show 命令用这个看磁盘上
// token 是否真的存在。
func readLLMRaw(path string) (config.LLM, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config.LLM{}, err
	}
	// 用完整 Config 解析再取 LLM 段（避免手写 toml 子文档解析）。
	cfg := config.Config{}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return config.LLM{}, err
	}
	return cfg.LLM, nil
}

func parseInt(s string) (int, error) {
	n := 0
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not int")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func emptyAs(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func maskKey(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
