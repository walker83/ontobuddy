// Package viz 把本体（Store）转换为交互式力导向图 HTML。
//
// 工作流：
//  1. 从 store 中取出实体（subjects）和关系三元组
//  2. 转成 vis-network 格式的 nodes/edges
//  3. 嵌入 HTML 模板（go:embed），注入数据，写到磁盘
//
// 模板用 vis-network 9.x（https://visjs.github.io/vis-network/），
// 从 CDN 加载 JS/CSS，保证单文件、可离线工作（首次加载后浏览器缓存）。
package viz

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"

	"github.com/walker/myonto/internal/rdf"
	"github.com/walker/myonto/internal/store"
)

// graphTemplate 是内嵌的 HTML 模板。
// 占位符 {{DATA}} 会被 JSON 替换；{{TITLE}} 是页面标题。
//
//go:embed template.html
var graphTemplate string

// Node 是 vis-network 的节点结构。
type Node struct {
	ID    string `json:"id"`              // 实体 IRI 的 local name（唯一）
	Label string `json:"label"`           // 显示文本（rdfs:label > local name）
	Group string `json:"group,omitempty"` // 用于着色（rdfs:Class 标 "class"，否则为类型 local name）
	Title string `json:"title,omitempty"` // 鼠标悬停提示
}

// Edge 是 vis-network 的边结构。
type Edge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Label  string `json:"label,omitempty"`
	Arrows string `json:"arrows,omitempty"`
}

// Build 从 Store 构建图数据（nodes + edges）。
//
// 过滤规则：
//   - 不画 rdfs:label / rdfs:comment / rdf:type 等元谓词（节点太多难看）
//   - 不画 rdfs:subClassOf 的边（用 group 颜色区分类与实例即可）
func Build(s *store.Store, opts BuildOptions) (nodes []Node, edges []Edge) {
	// 收集所有 subject（IRI），给每个分配一个 ID
	subjects := s.Subjects()
	nodes = make([]Node, 0, len(subjects))
	idOf := map[rdf.Term]string{}
	for _, subj := range subjects {
		id := subj.LocalName()
		idOf[subj] = id

		// 取 label
		label := id
		for _, t := range s.Query(rdf.Triple{Subject: subj, Predicate: rdf.Label}) {
			label = t.Object.Value
			break
		}

		// 决定分组：类用 "class"；实例用其第一个类型的 local name
		group := ""
		title := label
		isClass := false
		for _, t := range s.Query(rdf.Triple{Subject: subj, Predicate: rdf.Type}) {
			if t.Object.Equal(rdf.Class) {
				isClass = true
				group = "class"
			} else if group == "" && !isClass {
				group = t.Object.LocalName()
			}
		}

		// 描述作为 tooltip
		for _, t := range s.Query(rdf.Triple{Subject: subj, Predicate: rdf.Comment}) {
			desc := t.Object.Value
			if len(desc) > 120 {
				desc = desc[:120] + "…"
			}
			title = label + "\n" + desc
			break
		}

		_ = isClass
		nodes = append(nodes, Node{ID: id, Label: label, Group: group, Title: title})
	}

	// 画边：subject 在数据中的三元组（过滤元谓词）
	skipPreds := map[string]bool{
		"label":         true,
		"comment":       true,
		"type":          true,
		"subClassOf":    true,
		"subPropertyOf": true,
	}
	if opts.SkipPredicates != nil {
		for _, p := range opts.SkipPredicates {
			skipPreds[p] = true
		}
	}
	// 可选：只画特定类型的关系
	includePreds := map[string]bool{}
	if len(opts.IncludePredicates) > 0 {
		for _, p := range opts.IncludePredicates {
			includePreds[p] = true
		}
	}

	for _, t := range s.Triples() {
		// 必须 S/P 都是 IRI
		if t.Subject.Kind != rdf.KindIRI || t.Predicate.Kind != rdf.KindIRI {
			continue
		}
		predName := t.Predicate.LocalName()
		if skipPreds[predName] {
			continue
		}
		if len(includePreds) > 0 && !includePreds[predName] {
			continue
		}
		// 宾语必须是 IRI（我们只画实体到实体的边，不画字面量）
		if t.Object.Kind != rdf.KindIRI {
			continue
		}
		// 宾语必须也在数据集中
		fromID, ok1 := idOf[t.Subject]
		toID, ok2 := idOf[t.Object]
		if !ok1 || !ok2 {
			continue
		}
		edges = append(edges, Edge{
			From:   fromID,
			To:     toID,
			Label:  predName,
			Arrows: "to",
		})
	}
	return
}

// BuildOptions 控制 Build 的行为。
type BuildOptions struct {
	// SkipPredicates 在默认排除列表之外，再额外排除的谓词 local name。
	SkipPredicates []string
	// IncludePredicates 只画这些谓词的关系；空表示画所有（除默认排除）。
	IncludePredicates []string
}

// Render 渲染成完整 HTML（bytes）。
// data 是 nodes + edges 的 JSON 序列化（由调用方在外部序列化）。
func Render(title string, dataJSON []byte) ([]byte, error) {
	tpl := graphTemplate
	out := strings.Replace(tpl, "{{TITLE}}", htmlEscape(title), 1)
	// dataJSON 内嵌到 <script> 块中：必须转义会终止 <script> 的序列，
	// 否则实体的 label/comment 里出现 `</script>` 会提前结束脚本块，
	// 既破坏渲染又是 XSS 注入点（用户输入或 LLM extract 的内容都可能携带）。
	out = strings.Replace(out, "{{DATA}}", safeJSONForHTML(dataJSON), 1)
	return []byte(out), nil
}

// safeJSONForHTML 把 JSON 文本转义成可安全内嵌进 HTML <script> 的形式。
//
// JSON 规范允许字符串里出现任意字符（含 `</`），但 HTML 解析器看到 `</script>`
// 就会终止脚本块。标准做法是把 `<` 转成 `\u003c`——JSON 字符串里 `\uXXXX` 仍合法，
// JSON.parse 会还原成原字符，但 HTML 解析器再也看不到 `</script>`。
func safeJSONForHTML(data []byte) string {
	s := string(data)
	s = strings.ReplaceAll(s, "<", "\\u003c")
	s = strings.ReplaceAll(s, ">", "\\u003e")
	s = strings.ReplaceAll(s, "\u2028", "\\u2028") // JS 行分隔符，会断行
	s = strings.ReplaceAll(s, "\u2029", "\\u2029") // JS 段分隔符
	return s
}

// htmlEscape 对 HTML 标题做最小转义（防 XSS）。
func htmlEscape(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		switch r {
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '&':
			b.WriteString("&amp;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#39;")
		default:
			fmt.Fprintf(&b, "%c", r)
		}
	}
	return b.String()
}
