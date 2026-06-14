package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/walker/myonto/internal/llm"
	"github.com/walker/myonto/internal/rdf"
)

// startMockLLM 启动一个 mock OpenAI 兼容端点，返回由 handler 决定的响应。
func startMockLLM(t *testing.T, handler http.HandlerFunc) (string, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	return srv.URL, srv.Close
}

func aiSummaryHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := llm.ChatResponse{ID: "mock", Model: req.Model}
		resp.Choices = []struct {
			Index   int         `json:"index"`
			Message llm.Message `json:"message"`
			Finish  string      `json:"finish_reason"`
		}{{
			Index:   0,
			Message: llm.Message{Role: "assistant", Content: "Mock summary: 这是个测试实体。"},
		}}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func aiExtractHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		turtle := "@prefix ex: <http://example.org/> .\nex:alice a ex:Person ;\n    rdfs:label \"Alice\" ."
		resp := llm.ChatResponse{ID: "mock", Model: req.Model}
		resp.Choices = []struct {
			Index   int         `json:"index"`
			Message llm.Message `json:"message"`
			Finish  string      `json:"finish_reason"`
		}{{
			Index:   0,
			Message: llm.Message{Role: "assistant", Content: "```turtle\n" + turtle + "\n```"},
		}}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// makeOntology 创建一个临时本体库，返回其目录路径。
func makeOntology(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "ontology.ttl")
	if err := os.WriteFile(dataPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, ".myonto.toml")
	cfg := "base_iri = \"http://example.org/\"\ndata_file = \"ontology.ttl\"\nprefix = \"ex\"\n\n"
	cfg += "[llm]\nbase_url = \"PLACEHOLDER\"\napi_key = \"test\"\nmodel = \"mock\"\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runInDir 切换到 dir 后跑 cli.RunArgs(args)。
func runInDir(t *testing.T, dir string, args []string) error {
	t.Helper()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	// 重置包级 JSON 标记，避免前一个测试的 --json 状态污染下一个测试。
	asJSON = false
	return RunArgs(args)
}

// TestAISummarize_DryRun 验证 dry-run 不写回。
func TestAISummarize_DryRun(t *testing.T) {
	baseURL, cleanup := startMockLLM(t, aiSummaryHandler(t))
	defer cleanup()

	dir := makeOntology(t, "@prefix ex: <http://example.org/> .\nex:alice rdfs:label \"Alice\" .\n")
	cfgBytes, _ := os.ReadFile(filepath.Join(dir, ".myonto.toml"))
	cfgStr := strings.Replace(string(cfgBytes), "PLACEHOLDER", baseURL, 1)
	os.WriteFile(filepath.Join(dir, ".myonto.toml"), []byte(cfgStr), 0o644)

	if err := runInDir(t, dir, []string{"ai", "summarize", "alice"}); err != nil {
		t.Fatalf("RunArgs 错误: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "ontology.ttl"))
	if strings.Contains(string(data), "Mock summary") {
		t.Error("dry-run 不应写回 rdfs:comment")
	}
}

// TestAISummarize_Apply 验证 -a 写回。
func TestAISummarize_Apply(t *testing.T) {
	baseURL, cleanup := startMockLLM(t, aiSummaryHandler(t))
	defer cleanup()

	dir := makeOntology(t, "@prefix ex: <http://example.org/> .\nex:alice rdfs:label \"Alice\" .\n")
	cfgBytes, _ := os.ReadFile(filepath.Join(dir, ".myonto.toml"))
	cfgStr := strings.Replace(string(cfgBytes), "PLACEHOLDER", baseURL, 1)
	os.WriteFile(filepath.Join(dir, ".myonto.toml"), []byte(cfgStr), 0o644)

	if err := runInDir(t, dir, []string{"ai", "summarize", "alice", "-a"}); err != nil {
		t.Fatalf("RunArgs 错误: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "ontology.ttl"))
	if !strings.Contains(string(data), "Mock summary") {
		t.Errorf("--apply 应把 LLM 输出写回 rdfs:comment，但文件: %s", data)
	}
}

// TestAIExtract_Apply 验证 extract 命令 + -a 把 Turtle 合并到本体。
func TestAIExtract_Apply(t *testing.T) {
	baseURL, cleanup := startMockLLM(t, aiExtractHandler(t))
	defer cleanup()

	dir := makeOntology(t, "@prefix ex: <http://example.org/> .\n")
	cfgBytes, _ := os.ReadFile(filepath.Join(dir, ".myonto.toml"))
	cfgStr := strings.Replace(string(cfgBytes), "PLACEHOLDER", baseURL, 1)
	os.WriteFile(filepath.Join(dir, ".myonto.toml"), []byte(cfgStr), 0o644)

	if err := runInDir(t, dir, []string{"ai", "extract", "牛是个学生", "-a"}); err != nil {
		t.Fatalf("RunArgs 错误: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "ontology.ttl"))
	if !strings.Contains(string(data), "Alice") {
		t.Errorf("extract 应把 ex:alice 写入本体，但文件: %s", data)
	}
}

// TestAI_NoConfig 验证未配置 LLM 时清晰报错。
func TestAI_NoConfig(t *testing.T) {
	dir := makeOntology(t, "@prefix ex: <http://example.org/> .\nex:alice rdfs:label \"Alice\" .\n")
	cfgStr := "base_iri = \"http://example.org/\"\ndata_file = \"ontology.ttl\"\nprefix = \"ex\"\n\n[llm]\nbase_url = \"\"\napi_key = \"\"\nmodel = \"\"\n"
	os.WriteFile(filepath.Join(dir, ".myonto.toml"), []byte(cfgStr), 0o644)

	err := runInDir(t, dir, []string{"ai", "summarize", "alice"})
	if err == nil {
		t.Fatal("期望 LLM 未配置错误")
	}
	if !strings.Contains(err.Error(), "未配置") {
		t.Errorf("错误信息应含 '未配置': %v", err)
	}
}

var _ = kong.Parse
var _ = rdf.IRI
var _ = llm.FromConfig
