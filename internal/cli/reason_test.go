package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// reasonFixture 创建一个有类继承的最小本体，便于跑推理。
func reasonFixture(t *testing.T) string {
	t.Helper()
	toml := `@prefix ex: <http://example.org/> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .

ex:Person a rdfs:Class .
ex:Scientist a rdfs:Class ; rdfs:subClassOf ex:Person .
ex:newton a ex:Scientist .
`
	return makeOntology(t, toml)
}

// captureStdout 跑 fn 并捕获它写到 stdout 的内容。
// 用临时文件而不是 pipe，避免某些输出带特殊字符时 io 缓冲问题。
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	tmp, _ := os.CreateTemp("", "stdout-*.txt")
	tmp.Close()
	defer os.Remove(tmp.Name())

	oldStdout := os.Stdout
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	_ = devnull.Close()

	// 直接重定向到文件
	f, _ := os.Create(tmp.Name())
	os.Stdout = f
	fn()
	f.Close()
	os.Stdout = oldStdout

	data, _ := os.ReadFile(tmp.Name())
	return string(data)
}

// TestReason_JSON_SaturatedFalse 验证 --json 输出结构（含 derived[]）。
func TestReason_JSON_SaturatedFalse(t *testing.T) {
	dir := reasonFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"reason", "--json"})
	})

	var got reasonResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	if got.Saturated {
		t.Error("Saturated 应为 false（本体还有可推论）")
	}
	if len(got.Derived) == 0 {
		t.Error("Derived 应非空")
	}
	// 期望：newton a Person（类型继承）
	found := false
	for _, tr := range got.Derived {
		if tr.Subject == "http://example.org/newton" &&
			tr.Predicate == "http://www.w3.org/1999/02/22-rdf-syntax-ns#type" {
			if obj, ok := tr.Object["value"].(string); ok && obj == "http://example.org/Person" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("应推出 newton a Person，实际: %+v", got.Derived)
	}
	if got.WillApply {
		t.Error("WillApply 应为 false（没传 -a）")
	}
}

// TestReason_JSON_ApplyMaterializes 验证 -a 时 Applied 字段被填 + 文件落盘。
func TestReason_JSON_ApplyMaterializes(t *testing.T) {
	dir := reasonFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"reason", "--json", "--apply"})
	})

	var got reasonResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	if got.Applied == 0 {
		t.Errorf("Applied 应非零（已物化），got %d", got.Applied)
	}
	ttltxt, _ := os.ReadFile(filepath.Join(dir, "ontology.ttl"))
	if !strings.Contains(string(ttltxt), "Person") {
		t.Errorf("物化后本体应含 Person 关系，实际文件: %s", ttltxt)
	}
}

// TestReason_JSON_SaturatedTrue 验证已饱和时 Saturated=true / Derived=空。
func TestReason_JSON_SaturatedTrue(t *testing.T) {
	dir := reasonFixture(t)
	// 第一次跑物化推论
	captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"reason", "--apply"})
	})

	// 第二次跑应报告饱和
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"reason", "--json"})
	})
	var got reasonResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	if !got.Saturated {
		t.Errorf("Saturated 应为 true（已饱和），输出: %s", out)
	}
	if len(got.Derived) != 0 {
		t.Errorf("Derived 应为空，got %d", len(got.Derived))
	}
}

// TestReason_HumanReadable 验证不加 --json 时走文本路径。
func TestReason_HumanReadable(t *testing.T) {
	dir := reasonFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"reason"})
	})
	if !strings.Contains(out, "推理共推导出") {
		t.Errorf("人类可读输出应含「推理共推导出」，实际: %s", out)
	}
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Error("未传 --json 时不应输出 JSON")
	}
}

// 确保引用还在
var _ = bytes.NewBuffer

// TestReason_ApplyWithLimit_MaterializesAll 是 H1 的回归测试：
// `reason -a -n <limit>` 展示的三元组被截断，但物化必须写入全量。
// 修复前 derived 切片被复用做"展示"和"物化"两件事，会只写前 limit 条。
func TestReason_ApplyWithLimit_MaterializesAll(t *testing.T) {
	// 3 层继承 + 3 个实例：每个实例推出 2 条类型继承（a Mammal, a Animal），
	// 加上 Primate⊑Animal 的传递闭包 1 条，共 7 条推导。
	toml := `@prefix ex: <http://example.org/> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .

ex:Animal a rdfs:Class .
ex:Mammal a rdfs:Class ; rdfs:subClassOf ex:Animal .
ex:Primate a rdfs:Class ; rdfs:subClassOf ex:Mammal .

ex:newton a ex:Primate .
ex:darwin a ex:Primate .
ex:curie  a ex:Primate .
`
	dir := makeOntology(t, toml)

	// 先用 --json（不截断）拿到全量推导数作为基准。
	var baseline reasonResult
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"reason", "--json"})
	})
	if err := json.Unmarshal([]byte(out), &baseline); err != nil {
		t.Fatalf("解析 baseline JSON: %v", err)
	}
	if len(baseline.Derived) < 5 {
		t.Fatalf("fixture 应至少推出 5 条，实际 %d（fixture 设置有误）", len(baseline.Derived))
	}
	totalDerived := len(baseline.Derived)

	// 关键场景：人类可读模式 + apply + limit=2。
	// 修复前：只物化前 2 条；修复后：物化全量 totalDerived 条。
	captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"reason", "-a", "-n", "2"})
	})

	// 最强断言：重新加载后再跑一次推理，应报告「已饱和」。
	// 如果有任何一条推导没被物化，二次推理会再次推出它 → Derived 非空 → Saturated=false。
	// 这是验证"全量物化"最干净的方式，不依赖 Turtle 序列化格式。
	out2 := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"reason", "--json"})
	})
	var after reasonResult
	if err := json.Unmarshal([]byte(out2), &after); err != nil {
		t.Fatalf("解析 after JSON: %v\n输出: %s", err, out2)
	}
	if !after.Saturated || len(after.Derived) != 0 {
		t.Errorf("limit=2 apply 后应已物化全部 %d 条推导（二次推理应饱和），"+
			"但还有 %d 条未物化 → H1 修复失效\n输出: %s",
			totalDerived, len(after.Derived), out2)
	}
}
