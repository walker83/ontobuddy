package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/walker/myonto/internal/config"
	"github.com/walker/myonto/internal/rdf"
)

// TestStoreAddDedup 验证 Add 去重。
func TestStoreAddDedup(t *testing.T) {
	s := New(config.Default())
	tr := rdf.Triple{
		Subject:   rdf.IRI("http://example.org/a"),
		Predicate: rdf.Label,
		Object:    rdf.Lit("A"),
	}
	s.Add(tr)
	s.Add(tr) // 重复
	s.Add(tr) // 再重复
	if got := s.Len(); got != 1 {
		t.Errorf("去重后应剩 1 条，实际 %d", got)
	}
	if !s.Has(tr) {
		t.Error("Has 返回 false")
	}
}

// TestStoreRemove 验证按模式删除。
func TestStoreRemove(t *testing.T) {
	s := New(config.Default())
	subj := rdf.IRI("http://example.org/a")
	s.Add(rdf.Triple{Subject: subj, Predicate: rdf.Label, Object: rdf.Lit("A")})
	s.Add(rdf.Triple{Subject: subj, Predicate: rdf.Comment, Object: rdf.Lit("desc")})
	s.Add(rdf.Triple{Subject: rdf.IRI("http://example.org/b"), Predicate: rdf.Label, Object: rdf.Lit("B")})

	// 删除 a 的所有三元组（subject 通配谓词与宾语）。
	n := s.Remove(rdf.Triple{Subject: subj})
	if n != 2 {
		t.Errorf("应删除 2 条，实际 %d", n)
	}
	if s.Len() != 1 {
		t.Errorf("删除后应剩 1 条，实际 %d", s.Len())
	}
}

// TestStoreFileRoundTrip 验证 save -> load 不丢三元组。
func TestStoreFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.BaseIRI = "http://example.org/"
	s := New(cfg)

	subj := s.LocalIRI("newton")
	s.Add(rdf.Triple{Subject: subj, Predicate: rdf.Label, Object: rdf.Lit("Isaac Newton")})
	s.Add(rdf.Triple{Subject: subj, Predicate: rdf.Comment, Object: rdf.Lit("物理学家")})
	s.Add(rdf.Triple{Subject: subj, Predicate: rdf.Type, Object: s.LocalIRI("Person")})
	s.Add(rdf.Triple{Subject: subj, Predicate: s.LocalIRI("knows"), Object: s.LocalIRI("leibniz")})

	path := filepath.Join(dir, cfg.DataFile)
	if err := s.SaveFile(path); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 重新加载。
	s2 := New(cfg)
	if err := s2.LoadFile(path); err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if s2.Len() != s.Len() {
		t.Errorf("round-trip 后数量不一致: want %d, got %d", s.Len(), s2.Len())
	}
	for _, tr := range s.Triples() {
		if !s2.Has(tr) {
			t.Errorf("round-trip 丢失三元组: %s", tr)
		}
	}
}

// TestStoreLoadMissingFile 验证加载不存在的文件不报错。
func TestStoreLoadMissingFile(t *testing.T) {
	s := New(config.Default())
	err := s.LoadFile(filepath.Join(t.TempDir(), "nope.ttl"))
	if err != nil {
		t.Errorf("加载不存在的文件不应报错，得到: %v", err)
	}
	if s.Len() != 0 {
		t.Errorf("空 Store 应有 0 条，实际 %d", s.Len())
	}
}

// TestResolveName 验证名字解析的三种形式。
func TestResolveName(t *testing.T) {
	s := New(config.Default())

	// 裸 local name -> 本地命名空间。
	got, err := s.ResolveName("newton")
	if err != nil {
		t.Fatal(err)
	}
	if got.Value != "http://example.org/newton" {
		t.Errorf("裸名解析错误: got %s", got.Value)
	}

	// 前缀:local。
	got, err = s.ResolveName("ex:newton")
	if err != nil {
		t.Fatal(err)
	}
	if got.Value != "http://example.org/newton" {
		t.Errorf("前缀名解析错误: got %s", got.Value)
	}

	// 完整 IRI。
	got, err = s.ResolveName("<http://other.org/x>")
	if err != nil {
		t.Fatal(err)
	}
	if got.Value != "http://other.org/x" {
		t.Errorf("完整 IRI 解析错误: got %s", got.Value)
	}

	// 未知前缀应报错。
	_, err = s.ResolveName("unknown:x")
	if err == nil {
		t.Error("未知前缀应报错")
	}
}

// TestStoreQuery 验证模式查询的通配语义。
func TestStoreQuery(t *testing.T) {
	s := New(config.Default())
	a := s.LocalIRI("a")
	b := s.LocalIRI("b")
	s.Add(rdf.Triple{Subject: a, Predicate: rdf.Label, Object: rdf.Lit("A")})
	s.Add(rdf.Triple{Subject: a, Predicate: rdf.Comment, Object: rdf.Lit("desc")})
	s.Add(rdf.Triple{Subject: b, Predicate: rdf.Label, Object: rdf.Lit("B")})

	// 通配谓词与宾语。
	ts := s.Query(rdf.Triple{Subject: a})
	if len(ts) != 2 {
		t.Errorf("subject=a 应返回 2 条，实际 %d", len(ts))
	}

	// 通配 subject 与 object，按 predicate。
	ts = s.Query(rdf.Triple{Predicate: rdf.Label})
	if len(ts) != 2 {
		t.Errorf("predicate=label 应返回 2 条，实际 %d", len(ts))
	}

	// 精确匹配。
	ts = s.Query(rdf.Triple{Subject: a, Predicate: rdf.Comment, Object: rdf.Lit("desc")})
	if len(ts) != 1 {
		t.Errorf("精确匹配应返回 1 条，实际 %d", len(ts))
	}
}

// 确保使用 os 包（避免 lint 误报）。
var _ = os.Stat
