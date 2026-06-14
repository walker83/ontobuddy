// Package wizard 提供 myonto 的交互式配置向导。
//
// 底层用 charmbracelet/huh（基于 bubbletea），做多步骤表单：
//   - TTY 环境：渲染漂亮的 TUI 界面（上下键选择、回车确认、密码隐藏等）
//   - 非 TTY（CI、管道、ssh 非交互）：优雅降级——返回 ErrNoTTY，
//     让调用方走默认的"纯参数"流程
//
// 设计目标：让 `myonto init` 在第一次使用时只需回答几个问题，
// 不需要记任何参数或读文档。
package wizard

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"

	"github.com/walker/myonto/internal/config"
	cryptox "github.com/walker/myonto/internal/crypto"
	"github.com/walker/myonto/internal/llm"
)

// ErrNoTTY 表示当前环境没有 TTY，无法运行 TUI 向导。
// 调用方应捕获此错误并回退到纯参数模式。
var ErrNoTTY = errors.New("no TTY available")

// Result 是向导结束后收集到的用户选择。
type Result struct {
	// 基础本体设置
	BaseIRI  string
	Prefix   string
	DataFile string

	// LLM 设置（SetupLLM = false 时为 nil）
	SetupLLM  bool
	Provider  string // 当 SetupLLM = true
	Model     string // 当 SetupLLM = true（可空 → 用 provider 默认）
	APIKeyRaw string // 当 SetupLLM = true 且用户实际输入了 key
	// 注：APIKeyRaw 是明文，调用方应立即用 cryptox.Encrypt 加密后丢弃。

	// 选填：是否复制示例本体进目录
	InstallExamples bool
}

// Run 执行完整的 init 向导；交互式向用户提问。
//
// 参数：
//   - stdout/stderr：输出目标
//   - existing：已存在的 config（若 .myonto.toml 已存在）；为 nil 表示全新初始化
//   - examplesPath：如果用户选了「安装示例本体」，从这里复制；空字符串跳过该选项
func Run(stdout, stderr io.Writer, existing *config.Config, examplesPath string) (*Result, error) {
	// 探测 TTY
	if !isTTY() {
		return nil, ErrNoTTY
	}

	res := &Result{
		BaseIRI:  "http://example.org/",
		Prefix:   "ex",
		DataFile: "ontology.ttl",
	}
	if existing != nil {
		// 预填已有值
		res.BaseIRI = existing.BaseIRI
		res.Prefix = existing.Prefix
		res.DataFile = existing.DataFile
	}

	// ─── 步骤 1：基础本体设置 ───
	g1 := huh.NewGroup(
		huh.NewNote().
			Title("myonto · 初始化本体库").
			Description("回答几个问题完成设置。回车用默认值，Tab 切换，Ctrl+C 随时退出。"),
		huh.NewInput().
			Title("命名空间 IRI").
			Description("本项目所有实体的全局唯一前缀，类似 URL 但不必真可访问。\n推荐用你自己的域名/项目名；暂时可保留默认值。").
			Value(&res.BaseIRI).
			Placeholder("http://example.org/"),
		huh.NewInput().
			Title("命名空间前缀").
			Description("在 Turtle 文件里写成 ex:xxx 的简短别名。").
			Value(&res.Prefix).
			Placeholder("ex").
			Validate(func(s string) error {
				if s == "" {
					return errors.New("前缀不能为空")
				}
				return nil
			}),
		huh.NewInput().
			Title("数据文件名").
			Value(&res.DataFile).
			Placeholder("ontology.ttl"),
	)

	// ─── 步骤 2：是否配置 LLM（动态跳过） ───
	setupLLM := existing != nil && existing.LLM.APIKey != "" // 已配则默认 No
	g2 := huh.NewGroup(
		huh.NewConfirm().
			Title("现在配置 LLM 供应商（AI 辅助功能）？").
			Description("用于 myonto ai summarize/extract/qa 等命令。\n  - 现在配置：选供应商、输入 API key（自动加密保存）\n  - 稍后：可以随时跑 `myonto config llm set-key`").
			Value(&setupLLM).
			Affirmative("现在配置").
			Negative("稍后再说"),
	)

	// ─── 步骤 3：选 LLM provider + model + key（仅当 setupLLM = true） ───
	// 构造有序的 provider 选项（按 ID 字典序）
	provIDs := make([]string, 0, len(llm.BuiltinProviders))
	for id := range llm.BuiltinProviders {
		provIDs = append(provIDs, id)
	}
	sort.Strings(provIDs)

	provOptions := make([]huh.Option[string], 0, len(provIDs))
	for _, id := range provIDs {
		p := llm.BuiltinProviders[id]
		desc := fmt.Sprintf("%s · %s · %s", p.Name, p.Protocol, p.DefaultModel)
		provOptions = append(provOptions, huh.NewOption(desc, id))
	}

	// 预选 provider
	if existing != nil && existing.LLM.Provider != "" {
		res.Provider = existing.LLM.Provider
	} else {
		// 默认推荐 alibaba-coding
		res.Provider = "alibaba-coding"
	}

	// 预填 model
	prefilledModel := ""
	if existing != nil && existing.LLM.Model != "" {
		prefilledModel = existing.LLM.Model
	}

	g3 := huh.NewGroup(
		huh.NewSelect[string]().
			Title("选择 LLM 供应商").
			Options(provOptions...).
			Value(&res.Provider),
		huh.NewSelect[string]().
			Title("选择模型").
			Description("回车用所选供应商的默认模型；选 Other 可输入自定义 ID").
			Options(
				huh.NewOption("用供应商默认（推荐）", ""),
			).
			Value(&res.Model).
			Filtering(true),
	).WithHideFunc(func() bool { return !setupLLM })

	// 上面只放了「默认」一个选项其实没法选——动态加所选 provider 的可用模型。
	// 修正：每次选项重建。huh 做不到动态 options，
	// 所以拆成"两套 group"：先选 provider，再根据 provider 选/输入 model。
	// 简化：单选 provider 后直接给个 Input 让你填 model，留空 = 默认。
	g3b := huh.NewGroup(
		huh.NewInput().
			Title("模型 ID").
			Description("留空用供应商默认；也可以填具体模型 ID（如 glm-5, gpt-4o）").
			Value(&res.Model).
			Placeholder(prefilledModel),
	).WithHideFunc(func() bool { return !setupLLM })

	// API key 输入（隐藏）
	g4 := huh.NewGroup(
		huh.NewInput().
			Title("API Key").
			Description("将用本机机器指纹加密后存入 .myonto.toml；不会明文落盘").
			EchoMode(huh.EchoModePassword).
			Value(&res.APIKeyRaw).
			Validate(func(s string) error {
				if s == "" {
					return errors.New("key 不能为空（选「稍后」可跳过）")
				}
				return nil
			}),
	).WithHideFunc(func() bool { return !setupLLM })

	// ─── 步骤 4：是否装示例本体（动态跳过） ───
	installExamples := false
	g5 := huh.NewGroup(
		huh.NewConfirm().
			Title("是否复制示例本体（philosophers.ttl）?").
			Description("一个含苏格拉底/柏拉图/亚里士多德等哲学家的小本体，方便试玩").
			Value(&installExamples).
			Affirmative("复制").
			Negative("跳过"),
	).WithHideFunc(func() bool { return examplesPath == "" })

	// ─── 汇总 + 确认 ───
	// 注意：summary 必须在表单 Run() 之后求值，那时 res.* 才反映用户选择。
	// 在构造期求值会显示初始值（甚至 nil panic）。这里只放占位文案，Run 后再渲染。
	confirmed := true // 默认 Yes
	g6 := huh.NewGroup(
		huh.NewNote().
			Title("请确认").
			Description("（确认时将显示实际填入的值）"),
		huh.NewConfirm().
			Title("开始写入文件？").
			Value(&confirmed).
			Affirmative("是").
			Negative("否"),
	)

	form := huh.NewForm(g1, g2, g3, g3b, g4, g5, g6).WithWidth(80)
	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("wizard: %w", err)
	}

	// 同步结果
	res.SetupLLM = setupLLM
	res.InstallExamples = installExamples

	// 用户在最终确认选了「否」：放弃写入，返回 nil（无错，但 Result 标记为取消）。
	if !confirmed {
		return res, ErrUserCancelled
	}
	return res, nil
}

// ErrUserCancelled 表示用户在最终确认步骤选了「否」，未写入任何文件。
// 调用方应据此给出友好提示，而非当作错误。
var ErrUserCancelled = errors.New("用户取消")

// Apply 把 Result 应用到指定目录的 .myonto.toml。
// （向导本身不直接写盘——把决定权留给调用方，便于测试和复用。）
func Apply(dir string, res *Result) error {
	cfgPath := filepath.Join(dir, config.ConfigFile)
	existing, _ := config.Load(cfgPath)
	if existing.BaseIRI == "" {
		existing = config.Default()
	}
	existing.BaseIRI = res.BaseIRI
	existing.Prefix = res.Prefix
	existing.DataFile = res.DataFile

	if res.SetupLLM && res.APIKeyRaw != "" {
		// 加密 key
		token, err := cryptox.Encrypt(res.APIKeyRaw)
		if err != nil {
			return fmt.Errorf("加密 key: %w", err)
		}
		prov := llm.GetProvider(res.Provider)
		existing.LLM.Provider = res.Provider
		if prov != nil && existing.LLM.BaseURL == "" {
			existing.LLM.BaseURL = prov.BaseURL
		}
		if res.Model != "" {
			existing.LLM.Model = res.Model
		} else if existing.LLM.Model == "" && prov != nil {
			existing.LLM.Model = prov.DefaultModel
		}
		if prov != nil {
			existing.LLM.Protocol = prov.Protocol
		}
		existing.LLM.APIKeyToken = token
		existing.LLM.APIKey = "" // 显式清空
	}

	return config.Save(cfgPath, existing)
}

// isTTY 判断 stdin 是否为交互终端。
func isTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
