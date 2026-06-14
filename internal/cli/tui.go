package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"

	"github.com/walker/myonto/internal/config"
)

// TUICmd 进入 TUI 交互模式：主菜单 → 选动作 → 进入对应子界面。
//
// 这是 myonto 给"人类"用的入口。CLI 风格（myonto entity add ...）
// 给 LLM/Skills 用；TUI 风格给普通用户用。
type TUICmd struct{}

// Run 启动 TUI 主菜单循环。
func (c *TUICmd) Run() error {
	// 探测 TTY（不在 TTY 里跑 huh 会乱套）
	if !isTTY() {
		return errors.New("myonto tui 需要 TTY 环境；非交互场景请用 myonto <子命令>")
	}

	// 检查配置（首次提示用户去 init 或 setup）
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, _, err := config.Find(cwd); err != nil {
		// 没配置：先引导用户 init
		fmt.Fprintln(os.Stderr, "未发现 .myonto.toml，先帮你初始化：")
		// 直接调用 init.Run 走 wizard（如果非 TTY 走 plain）
		init := &InitCmd{}
		if err := init.Run(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "初始化完成！现在进入 TUI...")
	}

	// 主循环
	for {
		action, err := mainMenu()
		if err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				fmt.Fprintln(os.Stderr, "已退出 TUI。")
				return nil
			}
			return err
		}
		if action == "quit" {
			fmt.Fprintln(os.Stderr, "再见 👋")
			return nil
		}
		// 执行选中的动作；动作本身可能是"再开一个 huh 会话"
		err = runAction(action)
		switch {
		case err == nil:
			// 完成，回主菜单
		case errors.Is(err, huh.ErrUserAborted):
			// 子动作里按 Esc：静默回主菜单（非整体退出）
		default:
			// 动作出错（如"实体不存在""LLM 未配置"）：打印并回主菜单，
			// 而不是把整个 TUI 退出——用户通常还想继续做别的事。
			fmt.Fprintf(os.Stderr, "出错：%v\n", err)
		}
	}
}

// mainMenu 弹出主菜单，返回选中的 action ID。
func mainMenu() (string, error) {
	var choice string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("myonto · TUI 模式").
				Description("上下键选择，回车确认，Esc 退出。"),
			huh.NewSelect[string]().
				Title("要做什么？").
				Options(
					huh.NewOption("📋 列出所有实体", "list"),
					huh.NewOption("🔍 搜索实体", "search"),
					huh.NewOption("➕ 添加个体", "add"),
					huh.NewOption("🧬 添加类", "add-class"),
					huh.NewOption("🔗 建立关系", "link"),
					huh.NewOption("🧠 跑推理", "reason"),
					huh.NewOption("🌐 生成关系图", "graph"),
					huh.NewOption("🤖 AI 辅助（summarize/extract/qa）", "ai"),
					huh.NewOption("⚙️  重配置 (LLM / 命名空间)", "setup"),
					huh.NewOption("❌ 退出", "quit"),
				).
				Value(&choice),
		),
	).WithWidth(80)
	if err := form.Run(); err != nil {
		return "", err
	}
	return choice, nil
}

// runAction 派发到对应子动作。每个动作自己构造 huh 表单（如果是表单式）
// 或调子命令。返回的 error 会冒泡到主循环；nil 表示完成或用户取消。
func runAction(action string) error {
	switch action {
	case "list":
		return (&EntityListCmd{}).Run()
	case "search":
		return runSearch()
	case "add":
		return runAddEntity()
	case "add-class":
		return runAddClass()
	case "link":
		return runLink()
	case "reason":
		return (&ReasonCmd{}).Run()
	case "graph":
		return (&GraphCmd{}).Run()
	case "ai":
		return runAIMenu()
	case "setup":
		return (&ConfigSetupCmd{}).Run()
	}
	return nil
}

// runSearch 弹一个搜索词输入框，调用 SearchCmd。
func runSearch() error {
	var keyword string
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("搜索关键词").Value(&keyword).Placeholder("newton"),
	))
	if err := form.Run(); err != nil {
		return err
	}
	if keyword == "" {
		return nil
	}
	return (&SearchCmd{Keyword: keyword}).Run()
}

// runAddEntity 弹一个表单收集 name/type/desc，调 EntityAddCmd。
func runAddEntity() error {
	var name, typ, desc string
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("实体名").Value(&name).Placeholder("Newton"),
		huh.NewInput().Title("类型（可空）").Value(&typ).Placeholder("Person"),
		huh.NewInput().Title("描述（可空）").Value(&desc),
	))
	if err := form.Run(); err != nil {
		return err
	}
	if name == "" {
		return nil
	}
	return (&EntityAddCmd{Name: name, Type: typ, Desc: desc}).Run()
}

// runAddClass 弹一个表单收集类名/父类/描述。
func runAddClass() error {
	var name, parent, desc string
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("类名").Value(&name).Placeholder("Scientist"),
		huh.NewInput().Title("父类（可空）").Value(&parent).Placeholder("Person"),
		huh.NewInput().Title("描述（可空）").Value(&desc),
	))
	if err := form.Run(); err != nil {
		return err
	}
	if name == "" {
		return nil
	}
	return (&EntityAddClassCmd{Name: name, Parent: parent, Desc: desc}).Run()
}

// runLink 弹一个表单收集 s/p/o，调 LinkCmd。
func runLink() error {
	var s, p, o string
	var asLiteral bool
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("主语").Value(&s).Placeholder("newton"),
		huh.NewInput().Title("谓词").Value(&p).Placeholder("knows"),
		huh.NewInput().Title("宾语").Value(&o).Placeholder("leibniz"),
		huh.NewConfirm().Title("宾语当字面量？").Value(&asLiteral).Affirmative("是").Negative("否（当 IRI 实体）"),
	))
	if err := form.Run(); err != nil {
		return err
	}
	if s == "" || p == "" || o == "" {
		return nil
	}
	return (&LinkCmd{Subject: s, Predicate: p, Object: o, Literal: asLiteral}).Run()
}

// runAIMenu 弹一个 AI 动作子菜单。
func runAIMenu() error {
	var choice string
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("AI 动作").
			Options(
				huh.NewOption("归纳实体：summarize <entity>", "summarize"),
				huh.NewOption("从文本抽取：extract <text>", "extract"),
				huh.NewOption("建议关系：suggest-relations <entity>", "suggest"),
				huh.NewOption("问答：qa <question>", "qa"),
			).
			Value(&choice),
	))
	if err := form.Run(); err != nil {
		return err
	}

	// 接下来弹主语/问题/文本输入
	var input string
	var askerLabel string
	switch choice {
	case "summarize", "suggest":
		askerLabel = "实体名"
	case "qa":
		askerLabel = "问题"
	case "extract":
		askerLabel = "自然语言文本"
	}
	form2 := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title(askerLabel).Value(&input),
	))
	if err := form2.Run(); err != nil {
		return err
	}
	if input == "" {
		return nil
	}

	switch choice {
	case "summarize":
		return (&AISummarizeCmd{Entity: input}).Run()
	case "extract":
		return (&AIExtractCmd{Text: input}).Run()
	case "suggest":
		return (&AISuggestRelationsCmd{Entity: input}).Run()
	case "qa":
		return (&AIQACmd{Question: input}).Run()
	}
	return nil
}

// TUI 用到的 isTTY 复用 config 包探测，但用 term 包更直接。
// 这里是本地副本以避免 import 冲突。
func isTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
