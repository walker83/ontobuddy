package cli

import (
	"fmt"
	"os"
	"reflect"

	"github.com/alecthomas/kong"
)

// CLI 是顶层命令树。
type CLI struct {
	// 全局 flag（不属于任何子命令）
	JSON bool `help:"以 JSON 格式输出（供 LLM/脚本解析）。不影响 stderr/交互；仅当命令支持时生效"`

	Init    InitCmd    `cmd:"" help:"在当前目录初始化一个本体库"`
	Entity  EntityCmd  `cmd:"" help:"管理实体（个体/类）的增删改查"`
	Link    LinkCmd    `cmd:"" help:"在两个实体之间建立关系（三元组）"`
	Unlink  UnlinkCmd  `cmd:"" help:"删除两个实体之间的关系（三元组）"`
	Search  SearchCmd  `cmd:"" help:"按关键词全文检索实体"`
	Schema  SchemaCmd  `cmd:"" help:"输出当前本体的元模型（类/谓词/约束），供 LLM 自省"`
	Import  ImportCmd  `cmd:"" help:"从 md/txt 文件提取结构化 JSON 草稿（不调 LLM）"`
	Export  ExportCmd  `cmd:"" help:"导出当前本体（-l 给 LLM / -j JSON / 默认 Turtle）"`
	List    ListCmd    `cmd:"" help:"列出本体中的所有实体"`
	Reason  ReasonCmd  `cmd:"" help:"基于 RDFS/OWL 规则跑推理（subClassOf 传递/类型继承/传递属性等）"`
	Check   CheckCmd   `cmd:"" help:"一致性检查：报告违反 owl:disjointWith 等约束的矛盾"`
	Closure ClosureCmd `cmd:"" help:"算某实体沿谓词的传递闭包（不物化，纯查询）"`
	Path    PathCmd    `cmd:"" help:"找两实体间的最短路径（BFS）"`
	Query   QueryCmd   `cmd:"" help:"轻量查询：三元组模式匹配 + GROUP BY/COUNT/Top-N（SPARQL 子集）"`
	Graph   GraphCmd   `cmd:"" help:"生成交互式力导向图（HTML），可自动打开浏览器"`
	Serve   ServeCmd   `cmd:"" help:"启动 Web UI 服务器（交互式图谱/规则/推理/检查）"`
	AI      AICmd      `cmd:"" help:"用 LLM 辅助整理本体（summarize/extract/suggest-relations/qa，默认 dry-run）"`
	Config  ConfigCmd  `cmd:"" help:"管理配置（如 LLM：set-key 加密、show、test、list-providers）"`
	TUI     TUICmd     `cmd:"" help:"进入 TUI 交互模式（主菜单/浏览/编辑）"`
	Version VersionCmd `cmd:"" help:"显示版本信息"`
}

// VersionCmd 打印版本。
type VersionCmd struct{}

// Run 实现 Kong 的命令派发。
func (c *VersionCmd) Run() error {
	fmt.Println("myonto dev (P0+P1)")
	return nil
}

// asJSON 是包级缓存：Run / RunArgs 把 Kong 解析后的 --json 值同步到这里，
// 让命令 Run() 内部用 IsJSON() 读。
var asJSON = false

// IsJSON 供各命令查询是否需要 JSON 输出。
func IsJSON() bool { return asJSON }

// syncJSON 从 kong 解析结果中读 --json 值，写入包级 asJSON。
//
// Kong 把 CLI struct 字段的 flag 值写回到传入的 &cli 指针指向的结构体。
// 所以我们用反射安全地提取它：先 deref 指针，再找 JSON 字段。
func syncJSON(target reflect.Value) {
	if !target.IsValid() {
		return
	}
	// target 是 reflect.ValueOf(&cli{})，需要 Elem() 拿 CLI struct
	v := target
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	f := v.FieldByName("JSON")
	if !f.IsValid() {
		return
	}
	if f.Kind() != reflect.Bool {
		return
	}
	asJSON = f.Bool()
}

// Run 是程序入口：解析 os.Args 并执行所选子命令。
//
// 当用户请求 --help、传错参数、或缺子命令时，Kong 会自行把帮助/错误
// 信息打印到 stderr 并调用 os.Exit。这是 Kong 的标准行为，对 CLI 程序
// 是用户预期的。本函数只在"正常运行完成"或"子命令 Run 返回错误"时返回。
func Run() error {
	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("myonto"),
		kong.Description("个人本体论管理助手 —— 用 RDF/OWL 标准管理你的知识"),
	)
	syncJSON(reflect.ValueOf(&cli))
	return ctx.Run()
}

// exitSignal 用于在 RunArgs 中把 kong.Exit 的调用转成可恢复的信号。
type exitSignal struct{ code int }

// RunArgs 用显式传入的 args 解析，主要供测试使用。
// 与 Run 不同：它不会在 help/错误时调 os.Exit，而是返回 error，
// 便于测试断言。
func RunArgs(args []string) (err error) {
	var cli CLI
	k, err := kong.New(&cli,
		kong.Name("myonto"),
		kong.Description("个人本体论管理助手 —— 用 RDF/OWL 标准管理你的知识"),
		kong.Writers(os.Stdout, os.Stderr),
		// 拦截 Exit：help 请求返回 nil（已打印帮助），其他返回 error。
		kong.Exit(func(code int) {
			panic(exitSignal{code})
		}),
	)
	if err != nil {
		return err
	}
	ctx, perr := safeParseAndRun(k, args)
	if perr != nil {
		return perr
	}
	// 从传入的 &cli 指针同步 --json 值
	syncJSON(reflect.ValueOf(&cli))
	if ctx == nil {
		return nil
	}
	// 测试模式下把 exitWith 换成 panic，并用 recover 捕获，
	// 使 path/check 等命令的 os.Exit(1) 不杀测试进程。
	origExit := exitWith
	exitWith = func(code int) { panic(exitSignal{code}) }
	defer func() {
		exitWith = origExit
		if r := recover(); r != nil {
			if sig, ok := r.(exitSignal); ok {
				if sig.code != 0 {
					err = fmt.Errorf("exit %d", sig.code)
				}
				return
			}
			panic(r)
		}
	}()
	return ctx.Run()
}

// safeParseAndRun 用 recover 捕获 kong.Exit 注入的 panic，转为正常返回。
func safeParseAndRun(k *kong.Kong, args []string) (ctx *kong.Context, err error) {
	defer func() {
		if r := recover(); r != nil {
			if sig, ok := r.(exitSignal); ok {
				if sig.code == 0 {
					return
				}
				err = fmt.Errorf("usage error (exit %d)", sig.code)
				return
			}
			panic(r)
		}
	}()
	ctx, err = k.Parse(args)
	if err != nil {
		return nil, err
	}
	return ctx, nil
}
