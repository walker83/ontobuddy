package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/walker/myonto/internal/viz"
)

// GraphCmd 生成交互式力导向图 HTML，可选自动开浏览器。
type GraphCmd struct {
	Output        string   `short:"o" help:"输出 HTML 路径，默认 ./ontology-graph.html" placeholder:"FILE"`
	Open          bool     `short:"O" help:"生成后自动在默认浏览器打开"`
	IncludePred   []string `help:"只画这些谓词的关系（local name），如 --include-pred knows --include-pred likes" placeholder:"PRED"`
	ExcludePred   []string `help:"额外排除的谓词（local name）" placeholder:"PRED"`
	Depth         int      `help:"仅展示距某实体指定深度的子图（暂未实现，保留参数）" placeholder:"N"`
	IncludeLabels []string `help:"仅展示 label 包含关键词的实体（多值取并集）" placeholder:"KEYWORD"`
}

// Run 生成图。
func (c *GraphCmd) Run() error {
	s, _, err := openStore()
	if err != nil {
		return err
	}

	nodes, edges := viz.Build(s, viz.BuildOptions{
		IncludePredicates: c.IncludePred,
		SkipPredicates:    c.ExcludePred,
	})
	if len(nodes) == 0 {
		fmt.Fprintln(os.Stdout, "（本体中无可视化内容）")
		return nil
	}

	// 可选：label 过滤
	if len(c.IncludeLabels) > 0 {
		nodes, edges = filterByLabel(nodes, edges, c.IncludeLabels)
		if len(nodes) == 0 {
			fmt.Fprintln(os.Stdout, "（过滤后无内容）")
			return nil
		}
	}

	// 序列化 nodes+edges
	payload := struct {
		Nodes []viz.Node `json:"nodes"`
		Edges []viz.Edge `json:"edges"`
	}{Nodes: nodes, Edges: edges}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	// 输出路径
	out := c.Output
	if out == "" {
		out = "ontology-graph.html"
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return err
	}

	// 渲染并写文件
	title := s.Config().BaseIRI
	html, err := viz.Render(title, data)
	if err != nil {
		return err
	}
	if err := os.WriteFile(abs, html, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "✓ 图已生成：%s\n", abs)
	fmt.Fprintf(os.Stdout, "  节点：%d，边：%d\n", len(nodes), len(edges))

	// 自动开浏览器
	if c.Open {
		if err := openInBrowser("file://" + abs); err != nil {
			fmt.Fprintf(os.Stderr, "自动打开浏览器失败：%v（手动打开文件即可）\n", err)
		}
	}
	return nil
}

// filterByLabel 保留 label 包含任一关键词的节点及关联边。
func filterByLabel(nodes []viz.Node, edges []viz.Edge, keywords []string) ([]viz.Node, []viz.Edge) {
	keep := map[string]bool{}
	for _, n := range nodes {
		for _, kw := range keywords {
			if containsCI(n.Label, kw) {
				keep[n.ID] = true
				break
			}
		}
	}
	filteredNodes := make([]viz.Node, 0, len(keep))
	for _, n := range nodes {
		if keep[n.ID] {
			filteredNodes = append(filteredNodes, n)
		}
	}
	filteredEdges := make([]viz.Edge, 0, len(edges))
	for _, e := range edges {
		if keep[e.From] && keep[e.To] {
			filteredEdges = append(filteredEdges, e)
		}
	}
	return filteredNodes, filteredEdges
}

func containsCI(s, sub string) bool {
	if sub == "" {
		return true
	}
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

// openInBrowser 跨平台打开 URL。
func openInBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return fmt.Errorf("不支持的 OS: %s", runtime.GOOS)
	}
	return cmd.Start()
}
