package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// pathFixture 建一个小型关联图：a knows b, b knows c, c knows d。
// 另有一条 a likes c（捷径测试用）。
func pathFixture(t *testing.T) string {
	t.Helper()
	toml := `@prefix ex: <http://example.org/> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .

ex:a ex:knows ex:b ; ex:likes ex:c .
ex:b ex:knows ex:c .
ex:c ex:knows ex:d .
`
	return makeOntology(t, toml)
}

// TestPath_Found 验证能找到最短路径。
// 图：a knows b, a likes c, b knows c, c knows d。
// a→d 的最短路径走 a likes c + c knows d（2 跳），不走 a knows b knows c knows d（3 跳）。
func TestPath_Found(t *testing.T) {
	dir := pathFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"path", "a", "d", "--json"})
	})
	var got pathResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	if !got.Found {
		t.Error("应找到路径")
	}
	// BFS 找最少边数：a likes c + c knows d = 2 跳
	if got.Length != 2 {
		t.Errorf("路径长度应为 2（走 likes 捷径），got %d", got.Length)
	}
	if len(got.Path) != 2 {
		t.Fatalf("Path 应有 2 条边，got %d", len(got.Path))
	}
	if !strings.HasSuffix(got.Path[0].Subject, "a") {
		t.Errorf("第一条边主语应为 a，got %s", got.Path[0].Subject)
	}
	if !strings.HasSuffix(got.Path[1].Object["value"].(string), "d") {
		t.Errorf("最后一条边宾语应为 d，got %v", got.Path[1].Object)
	}
}

// TestPath_DirectShortCut 验证优先走捷径（最短）。
// a likes c 是 1 跳，比 a knows b knows c（2 跳）短。
func TestPath_DirectShortCut(t *testing.T) {
	dir := pathFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"path", "a", "c", "--json"})
	})
	var got pathResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	// a likes c 是 1 跳直达
	if got.Length != 1 {
		t.Errorf("a→c 应走捷径 1 跳，got %d。路径: %+v", got.Length, got.Path)
	}
}

// TestPath_PredicateFilter 验证 --pred 限定谓词。
// 限定 knows 时 a→c 是 2 跳（绕过 likes 捷径）。
func TestPath_PredicateFilter(t *testing.T) {
	dir := pathFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"path", "a", "c", "-p", "knows", "--json"})
	})
	var got pathResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	// 限定 knows：a knows b knows c = 2 跳
	if got.Length != 2 {
		t.Errorf("限定 knows 时 a→c 应 2 跳，got %d", got.Length)
	}
}

// TestPath_NotFound 验证不连通时 Found=false 且 JSON 输出完整（exit code 由 RunArgs 恢复）。
func TestPath_NotFound(t *testing.T) {
	toml := `@prefix ex: <http://example.org/> .
ex:a ex:knows ex:b .
ex:c ex:knows ex:d .
`
	dir := makeOntology(t, toml)
	out := captureStdout(t, func() {
		// runInDir 返回的 error 含 "exit 1"，但 JSON 已在 os.Exit 前写完
		_ = runInDir(t, dir, []string{"path", "a", "c", "--json"})
	})
	var got pathResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败（os.Exit 应在 JSON 写完后触发）: %v\n输出: %s", err, out)
	}
	if got.Found {
		t.Error("a 与 c 不连通，Found 应为 false")
	}
	if got.Length != 0 {
		t.Errorf("未找到路径 Length 应为 0，got %d", got.Length)
	}
}

// TestPath_SameNode 验证 from==to 时长度 0。
func TestPath_SameNode(t *testing.T) {
	dir := pathFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"path", "a", "a", "--json"})
	})
	var got pathResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	if !got.Found {
		t.Error("同节点应 Found=true")
	}
	if got.Length != 0 {
		t.Errorf("同节点长度应为 0，got %d", got.Length)
	}
}

// TestPath_HumanReadable 验证人类可读输出。
func TestPath_HumanReadable(t *testing.T) {
	dir := pathFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"path", "a", "d"})
	})
	if !strings.Contains(out, "最短路径") {
		t.Errorf("应显示最短路径标题，实际: %s", out)
	}
	if !strings.Contains(out, "knows") {
		t.Errorf("应显示 knows 谓词，实际: %s", out)
	}
}
