package web

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/walker/myonto/internal/config"
	"github.com/walker/myonto/internal/rdf"
	"github.com/walker/myonto/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s := store.New(config.Config{})
	s.Add(rdf.Triple{Subject: rdf.IRI("ex:Cat"), Predicate: rdf.SubClassOf, Object: rdf.IRI("ex:Animal")})
	s.Add(rdf.Triple{Subject: rdf.IRI("ex:Felix"), Predicate: rdf.Type, Object: rdf.IRI("ex:Cat")})
	s.Add(rdf.Triple{Subject: rdf.IRI("ex:Felix"), Predicate: rdf.IRI("ex:name"), Object: rdf.Lit("Felix")})
	s.Add(rdf.Triple{Subject: rdf.IRI("ex:Dog"), Predicate: rdf.SubClassOf, Object: rdf.IRI("ex:Animal")})
	s.Add(rdf.Triple{Subject: rdf.IRI("ex:Cat"), Predicate: rdf.DisjointWith, Object: rdf.IRI("ex:Dog")})
	return s
}

// --- handleIndex ---

func TestHandleIndex(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("expected html content type, got %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "MyOntopo") {
		t.Error("expected HTML to contain 'MyOntopo'")
	}
}

func TestHandleIndex_NotFound(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleIndex_ExternalTemplate(t *testing.T) {
	dir := t.TempDir()
	webDir := filepath.Join(dir, ".myonto", "web")
	os.MkdirAll(webDir, 0o755)
	os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<html>CUSTOM</html>"), 0o644)

	s := newTestStore(t)
	srv := NewServer(s, Config{Dir: dir})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "CUSTOM") {
		t.Error("expected external template to be served")
	}
}

// --- handleRules ---

func TestHandleRules(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/rules", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var rules []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&rules); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(rules) != 9 {
		t.Errorf("expected 9 rules, got %d", len(rules))
	}
	// 验证规则字段
	for _, r := range rules {
		if _, ok := r["id"]; !ok {
			t.Error("rule missing 'id'")
		}
		if _, ok := r["name"]; !ok {
			t.Error("rule missing 'name'")
		}
		if _, ok := r["enabled"]; !ok {
			t.Error("rule missing 'enabled'")
		}
	}
}

func TestHandleRules_MethodNotAllowed(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("POST", "/api/rules", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- handleRuleToggle ---

func TestHandleRuleToggle_Enable(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	body := `{"enabled":false}`
	req := httptest.NewRequest("PUT", "/api/rules/subclass-transitive", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	if result["id"] != "subclass-transitive" {
		t.Errorf("expected id 'subclass-transitive', got %v", result["id"])
	}
	if result["enabled"] != false {
		t.Errorf("expected enabled false, got %v", result["enabled"])
	}
}

func TestHandleRuleToggle_NotFound(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	body := `{"enabled":false}`
	req := httptest.NewRequest("PUT", "/api/rules/nonexistent-rule", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleRuleToggle_InvalidJSON(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("PUT", "/api/rules/subclass-transitive", strings.NewReader("bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRuleToggle_EmptyID(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	body := `{"enabled":false}`
	req := httptest.NewRequest("PUT", "/api/rules/", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRuleToggle_MethodNotAllowed(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/rules/subclass-transitive", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- handleReason ---

func TestHandleReason(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("POST", "/api/reason", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if _, ok := result["derived"]; !ok {
		t.Error("expected 'derived' in response")
	}
	if _, ok := result["stats"]; !ok {
		t.Error("expected 'stats' in response")
	}
}

func TestHandleReason_MethodNotAllowed(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/reason", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- handleCheck ---

func TestHandleCheck(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("POST", "/api/check", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if _, ok := result["findings"]; !ok {
		t.Error("expected 'findings' in response")
	}
}

func TestHandleCheck_DisjointConflict(t *testing.T) {
	s := newTestStore(t)
	// Felix 同时是 Cat 和 Dog，Cat disjointWith Dog
	s.Add(rdf.Triple{Subject: rdf.IRI("ex:Felix"), Predicate: rdf.Type, Object: rdf.IRI("ex:Dog")})

	srv := NewServer(s, Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("POST", "/api/check", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	findings := result["findings"].([]interface{})
	if len(findings) == 0 {
		t.Error("expected disjointWith findings, got none")
	}
}

func TestHandleCheck_MethodNotAllowed(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/check", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- handleGraph ---

func TestHandleGraph(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/graph", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	nodes := result["nodes"].([]interface{})
	edges := result["edges"].([]interface{})
	if len(nodes) == 0 {
		t.Error("expected non-empty nodes")
	}
	if len(edges) == 0 {
		t.Error("expected non-empty edges")
	}
}

func TestHandleGraph_MethodNotAllowed(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("POST", "/api/graph", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- handleTriples ---

func TestHandleTriples_All(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/triples", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var triples []interface{}
	json.NewDecoder(w.Body).Decode(&triples)
	if len(triples) != s.Len() {
		t.Errorf("expected %d triples, got %d", s.Len(), len(triples))
	}
}

func TestHandleTriples_FilterSubject(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/triples?s=ex:Felix", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var triples []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&triples)
	for _, tr := range triples {
		subj := tr["Subject"].(map[string]interface{})
		if !strings.Contains(subj["Value"].(string), "Felix") {
			t.Errorf("expected Felix subject, got %v", subj["Value"])
		}
	}
}

func TestHandleTriples_MethodNotAllowed(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("POST", "/api/triples", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- handleStats ---

func TestHandleStats(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{})
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if _, ok := result["total_triples"]; !ok {
		t.Error("expected 'total_triples'")
	}
	if _, ok := result["inferred_triples"]; !ok {
		t.Error("expected 'inferred_triples'")
	}
	if _, ok := result["rule_stats"]; !ok {
		t.Error("expected 'rule_stats'")
	}
	if _, ok := result["timestamp"]; !ok {
		t.Error("expected 'timestamp'")
	}
}

// --- Start/Stop ---

func TestStartAndStop(t *testing.T) {
	s := newTestStore(t)
	srv := NewServer(s, Config{Port: 0}) // port 0 = auto assign

	addr, err := srv.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if !strings.HasPrefix(addr, "http://") {
		t.Errorf("expected http:// prefix, got %s", addr)
	}

	// 验证服务可访问
	resp, err := http.Get(addr + "/api/stats")
	if err != nil {
		t.Fatalf("GET /api/stats error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// 关闭
	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
}

func TestStartPortConflict_NextPort(t *testing.T) {
	// 占用一个端口
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // 立即释放，避免 isOwnServer 阻塞

	s := newTestStore(t)
	srv := NewServer(s, Config{Port: port})

	addr, err := srv.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer srv.Stop(context.Background())

	if !strings.HasPrefix(addr, "http://") {
		t.Errorf("expected http:// prefix, got %s", addr)
	}
}

// --- api.go ---

func TestBuildGraph_Empty(t *testing.T) {
	s := store.New(config.Config{})
	nodes, edges := buildGraph(s)
	if len(nodes) != 0 || len(edges) != 0 {
		t.Errorf("expected empty graph, got %d nodes, %d edges", len(nodes), len(edges))
	}
}

func TestQueryTriples_NoFilter(t *testing.T) {
	s := newTestStore(t)
	triples := queryTriples(s, "", "", "")
	if len(triples) != s.Len() {
		t.Errorf("expected %d, got %d", s.Len(), len(triples))
	}
}

func TestSearchTriples_EmptyQuery(t *testing.T) {
	s := newTestStore(t)
	triples := searchTriples(s, "")
	if len(triples) != s.Len() {
		t.Errorf("expected %d, got %d", s.Len(), len(triples))
	}
}

func TestSearchTriples_Match(t *testing.T) {
	s := newTestStore(t)
	triples := searchTriples(s, "Felix")
	if len(triples) == 0 {
		t.Error("expected matches for 'Felix'")
	}
}

func TestSearchTriples_NoMatch(t *testing.T) {
	s := newTestStore(t)
	triples := searchTriples(s, "NonexistentXYZ")
	if len(triples) != 0 {
		t.Errorf("expected 0, got %d", len(triples))
	}
}

func TestClassifyTerm_Literal(t *testing.T) {
	got := classifyTerm(rdf.Lit("hello"), rdf.Term{})
	if got != "literal" {
		t.Errorf("expected 'literal', got '%s'", got)
	}
}

func TestClassifyTerm_Class(t *testing.T) {
	got := classifyTerm(rdf.IRI("ex:Person"), rdf.Type)
	if got != "class" {
		t.Errorf("expected 'class', got '%s'", got)
	}
}

func TestClassifyTerm_Individual(t *testing.T) {
	got := classifyTerm(rdf.IRI("ex:alice"), rdf.IRI("ex:knows"))
	if got != "individual" {
		t.Errorf("expected 'individual', got '%s'", got)
	}
}
