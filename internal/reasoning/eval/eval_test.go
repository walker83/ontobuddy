package eval

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/walker/myonto/internal/rdf"
)

// TestRunCase_Case01_SubClassTransitive 验证 loader + harness + evaluate 全链路：
// 加载 case 01（subClassOf 传递），跑评估，应 P=R=F1=1.0 且 Pass。
// 如果此测试失败，说明：要么推理引擎坏了，要么 loader 解析 ttl 有问题，
// 要么 case 的期望三元组写错了——任何一环都会暴露。
func TestRunCase_Case01_SubClassTransitive(t *testing.T) {
	c, err := LoadCase(filepath.Join("testdata", "cases", "01-subclass-transitive"))
	if err != nil {
		t.Fatalf("LoadCase: %v", err)
	}
	if c.Name != "subclass-transitive" {
		t.Errorf("Name = %q, want subclass-transitive", c.Name)
	}
	if c.Rule != "subClassOf-transitive" {
		t.Errorf("Rule = %q", c.Rule)
	}
	if len(c.Input) != 2 {
		t.Errorf("Input 应有 2 条三元组，got %d", len(c.Input))
	}

	res := RunCase(c)
	if !res.Pass() {
		t.Errorf("case 应通过，实际:\n%s", res.String())
	}
	if res.TP != 1 {
		t.Errorf("TP 应为 1（A⊑C），got %d", res.TP)
	}
}

// TestEvaluate_Unit 直接测 evaluate 的集合运算逻辑，不依赖 reasoner 或文件 IO。
// 喂已知 derived，断言 TP/FP/FN 计算正确。
func TestEvaluate_Unit(t *testing.T) {
	c := Case{
		Name: "unit-test",
		Rule: "test",
		// 期望推出：[A, B]
		ExpectDerive: makeTripleSet("A", "B"),
		// 不应推出：[X]
		ExpectNotDerive: makeTripleSet("X"),
	}
	// 实际推出：[A, C, X] —— A 命中正例，X 命中负例(FP)，B 漏掉(FN)，C 是额外的(不计)
	derived := makeTripleSet("A", "C", "X")

	res := evaluate(c, derived)
	// TP=1 (A), FP=1 (X), FN=1 (B)
	if res.TP != 1 || res.FP != 1 || res.FN != 1 {
		t.Errorf("TP=%d FP=%d FN=%d, want 1/1/1", res.TP, res.FP, res.FN)
	}
	// P = 1/2 = 0.5, R = 1/2 = 0.5, F1 = 0.5
	if !floatEq(res.Precision, 0.5) || !floatEq(res.Recall, 0.5) || !floatEq(res.F1, 0.5) {
		t.Errorf("P=%.3f R=%.3f F1=%.3f, want 0.5/0.5/0.5", res.Precision, res.Recall, res.F1)
	}
	if res.Pass() {
		t.Error("有 FP+FN 不应 Pass")
	}
}

// TestComputeMetrics_EmptyCase 验证空 case（无期望无负例）的边界：P=R=1。
func TestComputeMetrics_EmptyCase(t *testing.T) {
	p, r, f1 := computeMetrics(0, 0, 0)
	if !floatEq(p, 1.0) || !floatEq(r, 1.0) || !floatEq(f1, 1.0) {
		t.Errorf("空 case 应 P=R=F1=1.0, got P=%.3f R=%.3f F1=%.3f", p, r, f1)
	}
}

// TestLoadCases_All 遍历 testdata/cases 下所有 case，逐个加载确保格式无误。
// 这是 case 文件本身的"语法 lint"——任何 ttl 写错、json 写错都会在这里炸。
func TestLoadCases_All(t *testing.T) {
	cases, err := LoadCases(filepath.Join("testdata", "cases"))
	if err != nil {
		t.Fatalf("LoadCases: %v", err)
	}
	if len(cases) < 12 {
		t.Errorf("应至少 12 个 case，got %d", len(cases))
	}
	t.Logf("加载了 %d 个 case", len(cases))
}

// TestRunAllCases 是 L2 的核心门槛：每个 golden case 都必须 P=R=F1=1.0。
// 任何一个 case 失败都会打印具体的漏推/错推三元组。
func TestRunAllCases(t *testing.T) {
	cases, err := LoadCases(filepath.Join("testdata", "cases"))
	if err != nil {
		t.Fatalf("LoadCases: %v", err)
	}
	if len(cases) < 12 {
		t.Fatalf("应至少 12 个 case，got %d", len(cases))
	}
	var results []Result
	for _, c := range cases {
		res := RunCase(c)
		results = append(results, res)
		// 每个 case 都打印，便于 CI 日志审查
		t.Log(res.String())
	}
	rep := Aggregate(results)
	t.Log(rep.String())
	if !rep.Pass() {
		t.Errorf("存在失败的 case（见上方明细）")
	}
}

// makeTripleSet 构造一组形如 ex:<local> rdfs:subClassOf ex:Root 的三元组，
// 用于单元测试中快速生成测试数据。
func makeTripleSet(locals ...string) []rdf.Triple {
	var ts []rdf.Triple
	for _, l := range locals {
		ts = append(ts, rdf.Triple{
			Subject:   rdf.IRI("http://example.org/" + l),
			Predicate: rdf.SubClassOf,
			Object:    rdf.IRI("http://example.org/Root"),
		})
	}
	return ts
}

// floatEq 容差比较浮点数（1e-9）。
func floatEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
