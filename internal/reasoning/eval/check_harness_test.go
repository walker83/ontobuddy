package eval

import (
	"path/filepath"
	"testing"

	"github.com/walker/myonto/internal/rdf"
	"github.com/walker/myonto/internal/reasoning"
)

// TestEvaluateCheck_Unit 直接测 evaluateCheck 的集合匹配逻辑，不依赖文件。
func TestEvaluateCheck_Unit(t *testing.T) {
	c := Case{
		Name: "unit",
		ExpectFindings: []ExpectedFinding{
			{Severity: "error", Rule: "owl:disjointWith", Subject: "felix"},
		},
	}
	// 实际：命中 felix + 多报一个 rex —— felix 命中，rex 计 extra
	actual := []reasoning.Finding{
		{Severity: "error", Rule: "owl:disjointWith", Subject: rdf.IRI("http://example.org/felix")},
		{Severity: "error", Rule: "owl:disjointWith", Subject: rdf.IRI("http://example.org/rex")},
	}
	res := evaluateCheck(c, actual)
	if res.Hit != 1 || len(res.Missed) != 0 || len(res.Extra) != 1 {
		t.Errorf("hit=1 missed=0 extra=1, got hit=%d missed=%d extra=%d", res.Hit, len(res.Missed), len(res.Extra))
	}
	if res.Extra[0].Subject.LocalName() != "rex" {
		t.Errorf("extra 应为 rex，got %s", res.Extra[0].Subject)
	}
	// 有 extra（多报）时 Pass() 必须为 false
	if res.Pass() {
		t.Error("有 extra 时不应 Pass")
	}
}

// TestEvaluateCheck_Missed 验证应报未报计入 missed。
func TestEvaluateCheck_Missed(t *testing.T) {
	c := Case{
		Name: "unit",
		ExpectFindings: []ExpectedFinding{
			{Severity: "error", Rule: "owl:disjointWith", Subject: "felix"},
		},
	}
	// 实际空：应报未报
	res := evaluateCheck(c, nil)
	if len(res.Missed) != 1 || res.Hit != 0 {
		t.Errorf("missed=1 hit=0, got missed=%d hit=%d", len(res.Missed), res.Hit)
	}
	if res.Pass() {
		t.Error("有 missed 不应 Pass")
	}
}

// TestSubjectMatches 验证 IRI 完整匹配和 local name 匹配两种模式。
func TestSubjectMatches(t *testing.T) {
	term := rdf.IRI("http://example.org/felix")
	if !subjectMatches(term, "http://example.org/felix") {
		t.Error("完整 IRI 应匹配")
	}
	if !subjectMatches(term, "felix") {
		t.Error("local name 应匹配")
	}
	if subjectMatches(term, "rex") {
		t.Error("不匹配的 local name 应返回 false")
	}
}

// TestRunCheckCases_Golden 是 check 评估的门槛：
// 遍历所有 case，对声明了 expect_findings 的跑 check 评估，全部必须 Pass。
func TestRunCheckCases_Golden(t *testing.T) {
	cases, err := LoadCases(filepath.Join("testdata", "cases"))
	if err != nil {
		t.Fatalf("LoadCases: %v", err)
	}
	ran := 0
	for _, c := range cases {
		if len(c.ExpectFindings) == 0 {
			continue
		}
		ran++
		res := RunCheckCase(c)
		t.Log(res.String())
		if !res.Pass() {
			t.Errorf("check case 失败: %s\n%s", c.Name, res.String())
		}
	}
	if ran == 0 {
		t.Log("（无 case 声明 expect_findings，跳过 check 评估）")
	}
}
