package viz

import (
	"strings"
	"testing"

	"github.com/walker/myonto/internal/config"
	"github.com/walker/myonto/internal/rdf"
	"github.com/walker/myonto/internal/store"
)

// newTestStore 构造一个测试用 store。
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	cfg := config.Default()
	cfg.BaseIRI = "http://example.org/"
	s := store.New(cfg)
	s.Add(rdf.Triple{Subject: s.LocalIRI("Person"), Predicate: rdf.Type, Object: rdf.Class})
	s.Add(rdf.Triple{Subject: s.LocalIRI("Person"), Predicate: rdf.Label, Object: rdf.Lit("Person")})
	s.Add(rdf.Triple{Subject: s.LocalIRI("alice"), Predicate: rdf.Type, Object: s.LocalIRI("Person")})
	s.Add(rdf.Triple{Subject: s.LocalIRI("alice"), Predicate: rdf.Label, Object: rdf.Lit("Alice")})
	s.Add(rdf.Triple{Subject: s.LocalIRI("bob"), Predicate: rdf.Type, Object: s.LocalIRI("Person")})
	s.Add(rdf.Triple{Subject: s.LocalIRI("alice"), Predicate: s.LocalIRI("knows"), Object: s.LocalIRI("bob")})
	return s
}

// TestBuild_BasicNodesAndEdges 验证基本节点/边生成。
func TestBuild_BasicNodesAndEdges(t *testing.T) {
	s := newTestStore(t)
	nodes, edges := Build(s, BuildOptions{})

	if len(nodes) != 3 { // Person, alice, bob
		t.Errorf("节点数 = %d, want 3", len(nodes))
	}
	if len(edges) != 1 { // alice knows bob
		t.Errorf("边数 = %d, want 1", len(edges))
	}
	if len(edges) > 0 {
		if edges[0].From != "alice" || edges[0].To != "bob" || edges[0].Label != "knows" {
			t.Errorf("边错误: %+v", edges[0])
		}
	}
}

// TestBuild_ClassGrouping 验证类被分到 "class" 组。
func TestBuild_ClassGrouping(t *testing.T) {
	s := newTestStore(t)
	nodes, _ := Build(s, BuildOptions{})

	var personNode *Node
	for i := range nodes {
		if nodes[i].ID == "Person" {
			personNode = &nodes[i]
			break
		}
	}
	if personNode == nil {
		t.Fatal("未找到 Person 节点")
	}
	if personNode.Group != "class" {
		t.Errorf("类应分组为 'class'，got %q", personNode.Group)
	}
}

// TestBuild_IncludePredicateFilter 验证 --include-pred 过滤。
func TestBuild_IncludePredicateFilter(t *testing.T) {
	s := newTestStore(t)
	// 加一条不会被包含的关系
	s.Add(rdf.Triple{Subject: s.LocalIRI("bob"), Predicate: s.LocalIRI("likes"), Object: s.LocalIRI("alice")})

	_, edges := Build(s, BuildOptions{IncludePredicates: []string{"knows"}})
	if len(edges) != 1 {
		t.Errorf("只画 knows 时应剩 1 条边，got %d", len(edges))
	}
	if len(edges) > 0 && edges[0].Label != "knows" {
		t.Errorf("过滤后边应是 knows，got %q", edges[0].Label)
	}
}

// TestBuild_SkipMetaPredicates 验证默认排除 label/comment/type/subClassOf。
func TestBuild_SkipMetaPredicates(t *testing.T) {
	s := newTestStore(t)
	// 加 subClassOf 关系（不应被画成边）
	s.Add(rdf.Triple{Subject: s.LocalIRI("Student"), Predicate: rdf.SubClassOf, Object: s.LocalIRI("Person")})
	// 加 label 关系（应被排除）
	s.Add(rdf.Triple{Subject: s.LocalIRI("alice"), Predicate: rdf.Label, Object: rdf.Lit("Alice")})

	_, edges := Build(s, BuildOptions{})
	for _, e := range edges {
		if e.Label == "label" || e.Label == "subClassOf" || e.Label == "type" {
			t.Errorf("不应画元谓词的边: %s", e.Label)
		}
	}
}

// TestBuild_LiteralObjectsSkipped 验证字面量宾语不会被画成边。
func TestBuild_LiteralObjectsSkipped(t *testing.T) {
	s := newTestStore(t)
	s.Add(rdf.Triple{Subject: s.LocalIRI("alice"), Predicate: s.LocalIRI("bornIn"), Object: rdf.Lit("Paris")})

	_, edges := Build(s, BuildOptions{})
	for _, e := range edges {
		if e.Label == "bornIn" {
			t.Error("字面量宾语不应被画为实体边")
		}
	}
}

// TestRender 验证 HTML 渲染：标题转义 + 数据注入。
func TestRender(t *testing.T) {
	_ = []Node{{ID: "a", Label: "A"}}
	_ = []Edge{{From: "a", To: "b", Label: "rel"}}
	data := []byte(`{"nodes":[{"id":"a","label":"A"}],"edges":[{"from":"a","to":"b","label":"rel"}]}`)

	html, err := Render(`<title>bad "xss" & chars</title>`, data)
	if err != nil {
		t.Fatal(err)
	}
	got := string(html)
	if !strings.Contains(got, "&lt;title&gt;") {
		t.Error("HTML 标题应被转义")
	}
	if !strings.Contains(got, `&amp;`) {
		t.Error("& 应被转义为 &amp;")
	}
	if !strings.Contains(got, `"id":"a"`) {
		t.Error("数据应被嵌入")
	}
	if !strings.Contains(got, "vis-network") {
		t.Error("应包含 vis-network 引用")
	}
}

// TestRender_DataScriptEscape 是 H5 的回归测试：data 里的 `</script>` 必须被转义，
// 否则会提前终止 <script> 块——既是渲染 bug 也是 XSS 注入点（label/comment
// 可能来自用户输入或 LLM extract）。
func TestRender_DataScriptEscape(t *testing.T) {
	// 模拟恶意 label：JSON 合法，但内嵌进 <script> 会破坏渲染。
	data := []byte(`{"nodes":[{"id":"x","label":"</script><script>alert(1)</script>"}]}`)
	html, err := Render("t", data)
	if err != nil {
		t.Fatal(err)
	}
	got := string(html)

	// 关键断言 1：原始的恶意序列不应原样出现。
	if strings.Contains(got, "</script><script>") {
		t.Errorf("data 中的 </script><script> 应被转义，原始序列仍存在\n%s", got)
	}
	// 关键断言 2：转义形式 \u003c/script\u003e 应存在（证明走的是转义路径）。
	if !strings.Contains(got, `\u003c/script\u003e`) {
		t.Errorf("应含转义后的 \\u003c/script\\u003e\n%s", got)
	}
	// 关键断言 3：注入的 alert(1) 不会以可执行形式出现。
	// 模板自身可能有合法的 <script> 标签（CDN 引用、数据块闭合），
	// 所以不能数 </script> 总数；而是确保注入 payload 不构成独立标签。
	if strings.Contains(got, "<script>alert(1)</script>") {
		t.Errorf("注入的 <script>alert(1)</script> 不应以可执行形式出现\n%s", got)
	}
}
