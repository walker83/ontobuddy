package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEntityRm_NoForceKeepsData 是 H2 的回归测试：
// `entity rm` 不带 -f 时应拒绝执行删除，本体文件原样不动。
//
// 修复前：先调 s.Remove 再判断 -f，删除发生在确认之前（虽因没 save 不落盘，
// 但内存 store 已被污染，且逻辑错误——一旦未来改为即时保存就会丢数据）。
func TestEntityRm_NoForceKeepsData(t *testing.T) {
	toml := `@prefix ex: <http://example.org/> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .

ex:newton a ex:Scientist ;
    rdfs:label "Isaac Newton" ;
    rdfs:comment "物理学家" .
`
	dir := makeOntology(t, toml)
	dataPath := filepath.Join(dir, "ontology.ttl")
	before, _ := os.ReadFile(dataPath)

	// 不带 -f：应返回 error，提示"加 -f 确认"。
	err := runInDir(t, dir, []string{"entity", "rm", "newton"})
	if err == nil {
		t.Fatal("rm 不带 -f 应返回 error 提示确认")
	}
	if !strings.Contains(err.Error(), "-f") {
		t.Errorf("error 应提示加 -f，实际: %v", err)
	}

	// 文件应原样未动（关键：没被删除污染）。
	after, _ := os.ReadFile(dataPath)
	if string(before) != string(after) {
		t.Errorf("rm 不带 -f 不应修改文件\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestEntityRm_ForceDeletes 是 H2 的对照测试：带 -f 时确实删除并落盘。
func TestEntityRm_ForceDeletes(t *testing.T) {
	toml := `@prefix ex: <http://example.org/> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .

ex:newton a ex:Scientist ;
    rdfs:label "Isaac Newton" .
ex:leibniz a ex:Scientist ;
    rdfs:label "Leibniz" .
`
	dir := makeOntology(t, toml)
	dataPath := filepath.Join(dir, "ontology.ttl")

	if err := runInDir(t, dir, []string{"entity", "rm", "-f", "newton"}); err != nil {
		t.Fatalf("rm -f 应成功，实际: %v", err)
	}
	after, _ := os.ReadFile(dataPath)
	body := string(after)
	if strings.Contains(body, "newton") {
		t.Errorf("rm -f 后文件不应再含 newton\n%s", body)
	}
	if !strings.Contains(body, "leibniz") {
		t.Errorf("rm -f newton 不应误删 leibniz\n%s", body)
	}
}

// TestEntityRm_NotFound 不存在的实体应报错且不动文件。
func TestEntityRm_NotFound(t *testing.T) {
	toml := `@prefix ex: <http://example.org/> .
ex:keep a ex:Thing .
`
	dir := makeOntology(t, toml)
	err := runInDir(t, dir, []string{"entity", "rm", "-f", "ghost"})
	if err == nil {
		t.Fatal("rm 不存在的实体应报错")
	}
}
