package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// queryFixture 建一个含多类型、多关系的本体供聚合测试。
// 4 个 Person（newton/leibniz/einstein/curie），2 个 Scientist（newton/einstein），
// 多个 knows 关系，bornIn 指向不同城市。
func queryFixture(t *testing.T) string {
	t.Helper()
	toml := `@prefix ex: <http://example.org/> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .

ex:Person a rdfs:Class .
ex:Scientist a rdfs:Class ; rdfs:subClassOf ex:Person .

ex:newton a ex:Scientist ; ex:knows ex:leibniz ; ex:bornIn ex:lincolnshire .
ex:leibniz a ex:Person ; ex:knows ex:newton ; ex:bornIn ex:leipzig .
ex:einstein a ex:Scientist ; ex:knows ex:curie ; ex:bornIn ex:ulm .
ex:curie a ex:Person ; ex:bornIn ex:warsaw .
`
	return makeOntology(t, toml)
}

// TestQuery_SimpleMatch 验证单模式匹配 + 投影。
func TestQuery_SimpleMatch(t *testing.T) {
	dir := queryFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"query", "-w", "?s a ex:Person", "--json"})
	})
	var got queryResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	// 注意：query 先 Derive，Scientist ⊑ Person ⟹ newton/einstein 也 a Person。
	// 所以 Person 含全部 4 个 + 推导出的 newton/einstein。
	names := map[string]bool{}
	for _, r := range got.Results {
		names[r.Key] = true
	}
	for _, want := range []string{"newton", "leibniz", "einstein", "curie"} {
		if !names[want] {
			t.Errorf("应含 %s（Person 实例，含继承推导），实际 %v", want, names)
		}
	}
}

// TestQuery_GroupByCount 验证 GROUP BY + COUNT 聚合。
// 按 rdf:type 分组，统计每个类有多少实例（含推导的类型）。
func TestQuery_GroupByCount(t *testing.T) {
	dir := queryFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"query", "-w", "?s a ?o", "-g", "?o", "-c", "--json"})
	})
	var got queryResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	counts := map[string]int{}
	for _, r := range got.Results {
		counts[r.Key] = r.Count
	}
	// Person: leibniz, curie 显式 + newton, einstein 推导 = 4
	if counts["Person"] != 4 {
		t.Errorf("Person 应 4 个实例（含推导），got %d", counts["Person"])
	}
	// Scientist: newton, einstein = 2
	if counts["Scientist"] != 2 {
		t.Errorf("Scientist 应 2 个实例，got %d", counts["Scientist"])
	}
}

// TestQuery_TopN 验证 --top 截断。
func TestQuery_TopN(t *testing.T) {
	dir := queryFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"query", "-w", "?s ex:bornIn ?o", "-g", "?o", "-c", "-n", "1", "--json"})
	})
	var got queryResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	if len(got.Results) != 1 {
		t.Errorf("Top 1 应只 1 行，got %d", len(got.Results))
	}
}

// TestQuery_JOIN 验证多模式 JOIN。
// ?s ex:knows ?o . ?o a ex:Person —— s 认识某个 Person。
func TestQuery_JOIN(t *testing.T) {
	dir := queryFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"query", "-w", "?s ex:knows ?o", "-w", "?o a ex:Person", "--json"})
	})
	var got queryResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	// newton knows leibniz(Person), einstein knows curie(Person), leibniz knows newton(Person)
	if len(got.Results) == 0 {
		t.Errorf("JOIN 应有结果，got 0。输出: %s", out)
	}
}

// TestQuery_EmptyResult 验证无匹配时输出空。
func TestQuery_EmptyResult(t *testing.T) {
	dir := queryFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"query", "-w", "?s a ex:Nonexistent", "--json"})
	})
	var got queryResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	if got.Total != 0 {
		t.Errorf("无匹配应 Total=0，got %d", got.Total)
	}
}

// TestQuery_HumanReadable 验证人类可读表格。
func TestQuery_HumanReadable(t *testing.T) {
	dir := queryFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"query", "-w", "?s a ?o", "-g", "?o", "-c"})
	})
	if !strings.Contains(out, "count") {
		t.Errorf("应显示 count 列，实际: %s", out)
	}
	if !strings.Contains(out, "Person") {
		t.Errorf("应显示 Person 分组，实际: %s", out)
	}
}

// TestQuery_Distinct 验证 --distinct 去重。
func TestQuery_Distinct(t *testing.T) {
	dir := queryFixture(t)
	// newton knows leibniz, leibniz knows newton —— 不去重 2 行，去重后仍是 2（不同的 ?s）
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"query", "-w", "?s ex:knows ?o", "-d", "--json"})
	})
	var got queryResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	// newton, einstein, leibniz 三个有 knows 出边
	if got.Total < 2 {
		t.Errorf("distinct 后应至少 2 个（newton/einstein/leibniz 有 knows），got %d", got.Total)
	}
}
