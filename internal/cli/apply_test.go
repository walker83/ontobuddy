package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/walker/myonto/internal/rdf"
)

// TestEntityApply_BatchEntities 验证批量加实体 + 关系。
func TestEntityApply_BatchEntities(t *testing.T) {
	dir := makeOntology(t, "@prefix ex: <http://example.org/> .\nex:Person a <http://www.w3.org/2000/01/rdf-schema#Class> .\n")
	// 写 JSON 输入
	jsonIn := `[{"name":"alice","type":"Person","desc":"A"},{"name":"bob","type":"Person"},{"subject":"alice","pred":"knows","object":"bob"}]`
	inPath := filepath.Join(dir, "in.json")
	os.WriteFile(inPath, []byte(jsonIn), 0o644)

	_ = runInDir(t, dir, []string{"entity", "apply", inPath})

	// 验证 alice / bob 都进了
	data, _ := os.ReadFile(filepath.Join(dir, "ontology.ttl"))
	ttl := string(data)
	if !strings.Contains(ttl, "alice") || !strings.Contains(ttl, "bob") {
		t.Errorf("本体应含 alice 和 bob，实际: %s", ttl)
	}
	if !strings.Contains(ttl, "knows") {
		t.Errorf("本体应含 knows 关系")
	}
}

// TestEntityApply_DryRun 验证 --dry 不写盘。
func TestEntityApply_DryRun(t *testing.T) {
	dir := makeOntology(t, "@prefix ex: <http://example.org/> .\nex:Person a <http://www.w3.org/2000/01/rdf-schema#Class> .\n")
	jsonIn := `[{"name":"alice","type":"Person"}]`
	inPath := filepath.Join(dir, "in.json")
	os.WriteFile(inPath, []byte(jsonIn), 0o644)

	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"entity", "apply", inPath, "--dry"})
	})
	if !strings.Contains(out, "dry-run") {
		t.Errorf("dry-run 应打印[dry-run]，实际: %s", out)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "ontology.ttl"))
	if strings.Contains(string(data), "alice") {
		t.Error("dry-run 不应写盘")
	}
}

// TestEntityApply_DuplicateSkipped 验证已存在的三元组被跳过。
func TestEntityApply_DuplicateSkipped(t *testing.T) {
	dir := makeOntology(t, "@prefix ex: <http://example.org/> .\nex:Person a <http://www.w3.org/2000/01/rdf-schema#Class> .\nex:alice rdfs:label \"alice\" .\n")
	jsonIn := `[{"name":"alice","type":"Person"}]`
	inPath := filepath.Join(dir, "in.json")
	os.WriteFile(inPath, []byte(jsonIn), 0o644)

	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"entity", "apply", inPath})
	})
	if !strings.Contains(out, "跳过") {
		t.Errorf("已存在应被跳过，实际: %s", out)
	}
}

// TestSchema_JSON_Output 验证 schema --json 输出结构。
func TestSchema_JSON_Output(t *testing.T) {
	dir := makeOntology(t, `@prefix ex: <http://example.org/> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix owl: <http://www.w3.org/2002/07/owl#> .
ex:Person a rdfs:Class .
ex:Animal a rdfs:Class ; rdfs:subClassOf ex:Person .
ex:knows a owl:ObjectProperty ; rdfs:domain ex:Person ; rdfs:range ex:Person .
`)

	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"schema", "--json"})
	})

	var m schemaModel
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("JSON 解析失败: %v\n输出: %s", err, out)
	}
	if len(m.Classes) < 2 {
		t.Errorf("应至少 2 个类，got %d", len(m.Classes))
	}
	// Animal 应有 parent Person
	foundAnimal := false
	for _, c := range m.Classes {
		if c.ID == "Animal" {
			foundAnimal = true
			if len(c.Parents) == 0 || c.Parents[0] != "Person" {
				t.Errorf("Animal 应有 parent Person, got %v", c.Parents)
			}
		}
	}
	if !foundAnimal {
		t.Error("未找到 Animal 类")
	}
	// Person 应有 children [Animal]
	foundPerson := false
	for _, c := range m.Classes {
		if c.ID == "Person" {
			foundPerson = true
			if len(c.Children) == 0 || c.Children[0] != "Animal" {
				t.Errorf("Person 应有 children [Animal], got %v", c.Children)
			}
		}
	}
	if !foundPerson {
		t.Error("未找到 Person 类")
	}
	// knows 谓词应被识别
	foundKnows := false
	for _, p := range m.Properties {
		if p.ID == "knows" {
			foundKnows = true
			if p.Domain != "Person" || p.Range != "Person" {
				t.Errorf("knows domain/range 错: %s → %s", p.Domain, p.Range)
			}
		}
	}
	if !foundKnows {
		t.Error("未找到 knows 谓词")
	}
}

// TestExport_ForLLM 验证 export --for-llm 输出紧凑文本。
func TestExport_ForLLM(t *testing.T) {
	dir := makeOntology(t, `@prefix ex: <http://example.org/> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
ex:Person a rdfs:Class .
ex:alice a ex:Person ; rdfs:label "Alice" ; rdfs:comment "测试" .
`)

	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"export", "--for-llm"})
	})
	if !strings.Contains(out, "## 类") || !strings.Contains(out, "## 个体") {
		t.Errorf("for-llm 输出应含类和个体段，实际: %s", out)
	}
	if !strings.Contains(out, "Alice") {
		t.Errorf("for-llm 输出应含 Alice 个体")
	}
}

// TestReason_ResetAfterApply 验证 reason --apply 后 --reset 能清掉推论。
func TestReason_ResetAfterApply(t *testing.T) {
	dir := makeOntology(t, `@prefix ex: <http://example.org/> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
ex:Person a rdfs:Class .
ex:Scientist a rdfs:Class ; rdfs:subClassOf ex:Person .
ex:newton a ex:Scientist .
`)

	// 物化推论
	_ = runInDir(t, dir, []string{"reason", "--apply"})

	// 本体应含推论 newton a Person
	data, _ := os.ReadFile(filepath.Join(dir, "ontology.ttl"))
	if !strings.Contains(string(data), "inferredBy") {
		t.Errorf("物化后应含 inferredBy 标记")
	}

	// reset 清除
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"reason", "--reset"})
	})
	if !strings.Contains(out, "清除") {
		t.Errorf("reset 应报告清除数量，实际: %s", out)
	}
	data2, _ := os.ReadFile(filepath.Join(dir, "ontology.ttl"))
	if strings.Contains(string(data2), "inferredBy") {
		t.Errorf("reset 后不应再含 inferredBy 标记")
	}
}

// 兜底 import
var _ = bytes.NewBuffer
var _ = rdf.IRI
