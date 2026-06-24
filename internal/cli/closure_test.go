package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// closureFixture 建一个传递依赖图：cmd→cli→store→rdf，cli→reasoning→rdf。
// dependsOn 声明为 owl:TransitiveProperty。
func closureFixture(t *testing.T) string {
	t.Helper()
	toml := `@prefix ex: <http://example.org/> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix owl: <http://www.w3.org/2002/07/owl#> .

ex:dependsOn a owl:TransitiveProperty .

ex:cmd ex:dependsOn ex:cli .
ex:cli ex:dependsOn ex:store ; ex:dependsOn ex:reasoning .
ex:store ex:dependsOn ex:rdf .
ex:reasoning ex:dependsOn ex:rdf .
`
	return makeOntology(t, toml)
}

// TestClosure_Forward 验证正向闭包：cmd 沿 dependsOn 应能到 store/reasoning/rdf。
func TestClosure_Forward(t *testing.T) {
	dir := closureFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"closure", "cmd", "-p", "dependsOn", "--json"})
	})
	var got closureResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	reachable := map[string]int{}
	for _, h := range got.Reachable {
		reachable[localOrFull(h.Term)] = h.Depth
	}
	// cmd → cli(1) → store(2)/reasoning(2) → rdf(3)
	for _, want := range []struct {
		node  string
		depth int
	}{
		{"cli", 1},
		{"store", 2},
		{"reasoning", 2},
		{"rdf", 3},
	} {
		d, ok := reachable[want.node]
		if !ok {
			t.Errorf("闭包应含 %s，实际 reachable=%v", want.node, reachable)
		} else if d != want.depth {
			t.Errorf("%s 深度应为 %d，got %d", want.node, want.depth, d)
		}
	}
	if got.Count != 4 {
		t.Errorf("Count 应为 4，got %d", got.Count)
	}
}

// TestClosure_Reverse 验证反向闭包：rdf 反向应到 store/reasoning/cli/cmd。
func TestClosure_Reverse(t *testing.T) {
	dir := closureFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"closure", "rdf", "-p", "dependsOn", "-r", "--json"})
	})
	var got closureResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	if !got.Reverse {
		t.Error("Reverse 应为 true")
	}
	reachable := map[string]bool{}
	for _, h := range got.Reachable {
		reachable[localOrFull(h.Term)] = true
	}
	for _, want := range []string{"store", "reasoning", "cli", "cmd"} {
		if !reachable[want] {
			t.Errorf("反向闭包应含 %s，实际 %v", want, reachable)
		}
	}
}

// TestClosure_DepthLimit 验证 --depth 截断。
func TestClosure_DepthLimit(t *testing.T) {
	dir := closureFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"closure", "cmd", "-p", "dependsOn", "-d", "1", "--json"})
	})
	var got closureResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	// 深度 1 只到 cli
	if got.Count != 1 {
		t.Errorf("深度 1 应只到 cli（Count=1），got %d", got.Count)
	}
}

// TestClosure_Empty 验证叶子节点闭包为空。
func TestClosure_Empty(t *testing.T) {
	dir := closureFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"closure", "rdf", "-p", "dependsOn", "--json"})
	})
	var got closureResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON parse 失败: %v\n输出: %s", err, out)
	}
	if got.Count != 0 {
		t.Errorf("rdf 无后继，Count 应为 0，got %d", got.Count)
	}
}

// TestClosure_HumanReadable 验证人类可读输出包含关键信息。
func TestClosure_HumanReadable(t *testing.T) {
	dir := closureFixture(t)
	out := captureStdout(t, func() {
		_ = runInDir(t, dir, []string{"closure", "cmd", "-p", "dependsOn"})
	})
	if !strings.Contains(out, "store") {
		t.Errorf("人类可读输出应含 store，实际: %s", out)
	}
	if !strings.Contains(out, "距离") {
		t.Errorf("应显示距离分组，实际: %s", out)
	}
}
