package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// checkFixture 创建一个含 disjointWith 冲突的本体。
func checkFixture(t *testing.T) string {
	t.Helper()
	toml := `@prefix ex: <http://example.org/> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix owl: <http://www.w3.org/2002/07/owl#> .

ex:Cat owl:disjointWith ex:Dog .
ex:felix a ex:Cat ; a ex:Dog .
`
	return makeOntology(t, toml)
}

// cleanCheckFixture 创建无冲突本体。
func cleanCheckFixture(t *testing.T) string {
	t.Helper()
	toml := `@prefix ex: <http://example.org/> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix owl: <http://www.w3.org/2002/07/owl#> .

ex:Cat owl:disjointWith ex:Dog .
ex:felix a ex:Cat .
ex:rex a ex:Dog .
`
	return makeOntology(t, toml)
}

// TestCheck_FindsConflict 验证人类可读输出能检测到 disjointWith 冲突。
func TestCheck_FindsConflict(t *testing.T) {
	dir := checkFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"check"})
	})
	if !strings.Contains(out, "felix") {
		t.Errorf("输出应提及 felix，实际: %s", out)
	}
	if !strings.Contains(out, "error") && !strings.Contains(out, "错误") {
		t.Errorf("输出应标明错误，实际: %s", out)
	}
	if !strings.Contains(out, "disjointWith") {
		t.Errorf("输出应提及 disjointWith 规则，实际: %s", out)
	}
}

// TestCheck_Clean 语义：无冲突时应输出通过信息。
func TestCheck_Clean(t *testing.T) {
	dir := cleanCheckFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"check"})
	})
	if !strings.Contains(out, "通过") {
		t.Errorf("无冲突应输出通过，实际: %s", out)
	}
}

// TestCheck_JSON 验证 --json 输出结构和 errors 计数。
func TestCheck_JSON(t *testing.T) {
	dir := checkFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"check", "--json"})
	})
	var got checkResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	if got.Errors != 1 {
		t.Errorf("Errors 应为 1，got %d", got.Errors)
	}
	if len(got.Findings) != 1 {
		t.Fatalf("应有 1 个 finding，got %d", len(got.Findings))
	}
	f := got.Findings[0]
	if f.Severity != "error" {
		t.Errorf("severity 应为 error，got %s", f.Severity)
	}
	if f.Rule != "owl:disjointWith" {
		t.Errorf("rule 应为 owl:disjointWith，got %s", f.Rule)
	}
	if !strings.HasSuffix(f.Subject, "felix") {
		t.Errorf("subject 应为 felix，got %s", f.Subject)
	}
	if len(f.Evidence) < 2 {
		t.Errorf("证据应至少 2 条（两条 type + 一条 disjointWith），got %d", len(f.Evidence))
	}
}

// TestCheck_JSON_Clean 无冲突时 --json 输出空 findings。
func TestCheck_JSON_Clean(t *testing.T) {
	dir := cleanCheckFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"check", "--json"})
	})
	var got checkResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	if got.Errors != 0 || len(got.Findings) != 0 {
		t.Errorf("无冲突应 errors=0 findings=[]，got errors=%d findings=%d", got.Errors, len(got.Findings))
	}
}

// TestCheck_InheritedConflict 验证经类型继承的隐式冲突也能被检测。
// felix a Kitten, Kitten ⊑ Cat, Cat disjointWith Dog, felix a Dog
// ⟹ felix 隐式 a Cat，与 a Dog 冲突。
func TestCheck_InheritedConflict(t *testing.T) {
	toml := `@prefix ex: <http://example.org/> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix owl: <http://www.w3.org/2002/07/owl#> .

ex:Cat owl:disjointWith ex:Dog .
ex:Kitten rdfs:subClassOf ex:Cat .
ex:felix a ex:Kitten ; a ex:Dog .
`
	dir := makeOntology(t, toml)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"check", "--json"})
	})
	var got checkResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	if got.Errors != 1 {
		t.Errorf("应检测到经继承的隐式冲突（errors=1），got %d。输出: %s", got.Errors, out)
	}
}
