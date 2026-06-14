package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/walker/myonto/internal/config"
	"github.com/walker/myonto/internal/wizard"
)

// InitCmd 实现 myonto init：首次交互式向导初始化；之后用纯参数模式。
//
// 智能检测：
//   - 当前目录无 .myonto.toml 且 stdin 是 TTY → 启动 TUI 向导
//   - 否则走原来的"纯参数"模式（保持脚本/远程部署可用）
//   - 加 --wizard 强制走向导
//   - 加 --no-wizard 强制走参数模式（即使在 TTY 下）
type InitCmd struct {
	BaseIRI  string `short:"i" help:"本地命名空间的基础 IRI，例如 http://myorg.org/。默认 http://example.org/" placeholder:"URI"`
	Prefix   string `short:"p" help:"本地命名空间的前缀（写 Turtle 时用），默认 ex" placeholder:"NAME"`
	Force    bool   `short:"f" help:"即使 .myonto.toml 已存在也强制覆盖"`
	DataFile string `help:"数据文件名，默认 ontology.ttl" placeholder:"FILE"`
	Dir      string `short:"d" help:"初始化的目标目录，默认当前目录" placeholder:"PATH"`
	Wizard   bool   `help:"强制走 TUI 向导（即使在非 TTY 也会尝试；非 TTY 会失败）"`
	NoWizard bool   `help:"强制走参数模式，不开 TUI"`
}

// Run 执行 init。
func (c *InitCmd) Run() error {
	dir := c.Dir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	cfgPath := filepath.Join(dir, config.ConfigFile)

	// 决定是否走向导
	shouldWizard, reason := c.shouldRunWizard(cfgPath)
	fmt.Fprintf(os.Stderr, "[myonto init] 向导模式: %v (%s)\n", shouldWizard, reason)

	if shouldWizard {
		return c.runWizard(dir, cfgPath)
	}
	return c.runPlain(dir, cfgPath)
}

// shouldRunWizard 决定是否走 TUI 向导。
//
// 真实 TTY 探测在 wizard.Run 内部进行；这里只做"是否启用向导"的策略判断。
// 实际命中非 TTY 时 wizard.Run 会返回 ErrNoTTY，触发回退。
func (c *InitCmd) shouldRunWizard(cfgPath string) (bool, string) {
	if c.Wizard {
		return true, "--wizard 显式启用"
	}
	if c.NoWizard {
		return false, "--no-wizard 显式禁用"
	}
	if _, err := os.Stat(cfgPath); err == nil {
		// 配置文件已存在：避免意外覆盖，向导不适用（用户应改用 `myonto setup`）
		return false, "配置文件已存在（用 myonto setup 重配置）"
	}
	return true, "首次初始化"
}

// runWizard 启动 TUI 向导。
func (c *InitCmd) runWizard(dir, cfgPath string) error {
	// 探测配置文件（向导在 cfgPath 不存在时传 nil；存在时让向导用它的值作默认值）
	var existing *config.Config
	if _, err := os.Stat(cfgPath); err == nil {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("读现有配置失败: %w", err)
		}
		existing = &cfg
	}

	// 找示例本体路径（如果项目里有 examples/）
	examplesPath := findExamples()

	res, err := wizard.Run(os.Stdout, os.Stderr, existing, examplesPath)
	if err != nil {
		if err == wizard.ErrNoTTY {
			fmt.Fprintln(os.Stderr, "无 TTY，自动回退到参数模式：")
			return c.runPlain(dir, cfgPath)
		}
		if err == wizard.ErrUserCancelled {
			fmt.Fprintln(os.Stdout, "已取消，未做任何修改。")
			return nil
		}
		return err
	}

	// 写入 .myonto.toml
	if err := wizard.Apply(dir, res); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "\n✓ 初始化完成：%s\n", cfgPath)

	// 复制示例本体（如果用户选了）
	if res.InstallExamples && examplesPath != "" {
		exampleName := "philosophers.ttl"
		dst := filepath.Join(dir, exampleName)
		if err := copyFile(examplesPath, dst); err != nil {
			fmt.Fprintf(os.Stderr, "复制示例本体失败: %v（可忽略）\n", err)
		} else {
			fmt.Fprintf(os.Stdout, "✓ 已复制示例本体到：%s\n", dst)
		}
	}

	// 提示下一步
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "接下来可以：")
	fmt.Fprintln(os.Stdout, "  myonto list                查看实体列表")
	fmt.Fprintln(os.Stdout, "  myonto entity add <名字>    添加一个实体")
	fmt.Fprintln(os.Stdout, "  myonto ai summarize <名>   试用 AI 辅助")
	return nil
}

// runPlain 原有的"纯参数"模式，行为与之前完全一致。
func (c *InitCmd) runPlain(dir, cfgPath string) error {
	// 检查是否已初始化。
	if _, err := os.Stat(cfgPath); err == nil && !c.Force {
		return fmt.Errorf("%s 已存在（用 --force 覆盖，或 --wizard 走向导重配置）", config.ConfigFile)
	}

	cfg := config.Default()
	if c.BaseIRI != "" {
		cfg.BaseIRI = c.BaseIRI
	}
	if c.Prefix != "" {
		cfg.Prefix = c.Prefix
	}
	if c.DataFile != "" {
		cfg.DataFile = c.DataFile
	}

	if err := config.Save(cfgPath, cfg); err != nil {
		return fmt.Errorf("写配置: %w", err)
	}

	dataPath := filepath.Join(dir, cfg.DataFile)
	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		empty := "@prefix " + cfg.Prefix + ": <" + cfg.BaseIRI + "> .\n"
		if err := os.WriteFile(dataPath, []byte(empty), 0o600); err != nil {
			return fmt.Errorf("写数据文件: %w", err)
		}
	}

	fmt.Fprintf(os.Stdout, "已初始化本体库：%s\n", dir)
	fmt.Fprintf(os.Stdout, "  配置文件：%s\n", config.ConfigFile)
	fmt.Fprintf(os.Stdout, "  数据文件：%s\n", cfg.DataFile)
	fmt.Fprintf(os.Stdout, "  命名空间：%s <%s>\n", cfg.Prefix, cfg.BaseIRI)
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "接下来可以用：")
	fmt.Fprintln(os.Stdout, "  myonto entity add-class <类名>   定义一个类")
	fmt.Fprintln(os.Stdout, "  myonto entity add <名字> -t <类> 添加一个实体")
	fmt.Fprintln(os.Stdout, "  myonto search <关键词>          搜索")
	return nil
}

// findExamples 查找 examples/philosophers.ttl 的路径。
//
// 策略：先看 cwd（开发模式用），再从可执行文件位置反推项目根。
// 找不到返回空字符串——向导会自动跳过「装示例」步骤。
func findExamples() string {
	candidates := []string{
		"examples/philosophers.ttl",
		"../examples/philosophers.ttl",
		"../../examples/philosophers.ttl",
		"../../../examples/philosophers.ttl",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	// 从可执行文件位置反推（go install 后场景）
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		// 尝试 ../examples/philosophers.ttl（如果 exe 在 bin/myonto）
		for i := 0; i < 4; i++ {
			try := filepath.Join(dir, "examples", "philosophers.ttl")
			if _, err := os.Stat(try); err == nil {
				return try
			}
			dir = filepath.Dir(dir)
		}
	}
	return ""
}

// copyFile 简单文件复制（避免引入额外工具）。
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}
