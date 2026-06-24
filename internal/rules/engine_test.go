package rules

import (
	"testing"

	"github.com/walker/myonto/internal/rdf"
)

func newTestCtx() *Context {
	return &Context{
		Transitive: map[rdf.Term]bool{},
		Symmetric:  map[rdf.Term]bool{},
		Inverses:   map[rdf.Term]rdf.Term{},
	}
}

func TestEngineDerive(t *testing.T) {
	rules, _ := LoadDefault()
	engine := NewEngine(rules, newTestCtx())

	base := []rdf.Triple{
		{Subject: rdf.IRI("ex:Cat"), Predicate: rdf.SubClassOf, Object: rdf.IRI("ex:Animal")},
		{Subject: rdf.IRI("ex:Felix"), Predicate: rdf.Type, Object: rdf.IRI("ex:Cat")},
	}

	result := engine.Derive(base)

	found := false
	for _, tr := range result.Derived {
		if tr.Subject.Equal(rdf.IRI("ex:Felix")) &&
			tr.Predicate.Equal(rdf.Type) &&
			tr.Object.Equal(rdf.IRI("ex:Animal")) {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected derived: Felix a Animal")
	}
	if len(result.Stats) == 0 {
		t.Error("expected non-empty stats")
	}
}

func TestEngineDisableRule(t *testing.T) {
	rules, _ := LoadDefault()
	for i, r := range rules {
		if r.ID == "type-inheritance" {
			rules[i].Enabled = false
		}
	}

	engine := NewEngine(rules, newTestCtx())
	base := []rdf.Triple{
		{Subject: rdf.IRI("ex:Cat"), Predicate: rdf.SubClassOf, Object: rdf.IRI("ex:Animal")},
		{Subject: rdf.IRI("ex:Felix"), Predicate: rdf.Type, Object: rdf.IRI("ex:Cat")},
	}

	result := engine.Derive(base)
	for _, tr := range result.Derived {
		if tr.Subject.Equal(rdf.IRI("ex:Felix")) &&
			tr.Predicate.Equal(rdf.Type) &&
			tr.Object.Equal(rdf.IRI("ex:Animal")) {
			t.Error("type-inheritance should be disabled, but Felix a Animal was derived")
		}
	}
}

func TestEngineStats(t *testing.T) {
	rules, _ := LoadDefault()
	engine := NewEngine(rules, newTestCtx())

	base := []rdf.Triple{
		{Subject: rdf.IRI("ex:A"), Predicate: rdf.SubClassOf, Object: rdf.IRI("ex:B")},
		{Subject: rdf.IRI("ex:B"), Predicate: rdf.SubClassOf, Object: rdf.IRI("ex:C")},
	}

	result := engine.Derive(base)
	if s, ok := result.Stats["subclass-transitive"]; ok {
		if s.ProducedTriples != 1 {
			t.Errorf("expected 1 produced triple, got %d", s.ProducedTriples)
		}
		if s.Iterations == 0 {
			t.Error("expected at least 1 iteration")
		}
	} else {
		t.Error("missing stats for subclass-transitive")
	}
}

func TestEngineUpdateRules(t *testing.T) {
	rules, _ := LoadDefault()
	engine := NewEngine(rules, newTestCtx())

	// 初始应该是 9 条
	if len(engine.Rules()) != 9 {
		t.Errorf("expected 9 rules, got %d", len(engine.Rules()))
	}

	// 更新为只有 2 条
	newRules := []RuleDef{
		{ID: "a", Name: "A", Enabled: true, Spec: RuleSpec{Type: "builtin", BuiltinID: "subclass-transitive"}},
	}
	engine.UpdateRules(newRules)
	if len(engine.Rules()) != 1 {
		t.Errorf("expected 1 rule after update, got %d", len(engine.Rules()))
	}
}

func TestEngineEmptyBase(t *testing.T) {
	rules, _ := LoadDefault()
	engine := NewEngine(rules, newTestCtx())

	result := engine.Derive(nil)
	if len(result.Derived) != 0 {
		t.Errorf("expected 0 derived from empty base, got %d", len(result.Derived))
	}
}

func TestEngineTransitiveProperty(t *testing.T) {
	ctx := &Context{
		Transitive: map[rdf.Term]bool{rdf.IRI("ex:partOf"): true},
		Symmetric:  map[rdf.Term]bool{},
		Inverses:   map[rdf.Term]rdf.Term{},
	}
	rules, _ := LoadDefault()
	engine := NewEngine(rules, ctx)

	base := []rdf.Triple{
		{Subject: rdf.IRI("ex:A"), Predicate: rdf.IRI("ex:partOf"), Object: rdf.IRI("ex:B")},
		{Subject: rdf.IRI("ex:B"), Predicate: rdf.IRI("ex:partOf"), Object: rdf.IRI("ex:C")},
	}

	result := engine.Derive(base)
	found := false
	for _, tr := range result.Derived {
		if tr.Subject.Equal(rdf.IRI("ex:A")) &&
			tr.Predicate.Equal(rdf.IRI("ex:partOf")) &&
			tr.Object.Equal(rdf.IRI("ex:C")) {
			found = true
		}
	}
	if !found {
		t.Error("expected transitive closure: A partOf C")
	}
}

func TestEngineSymmetricProperty(t *testing.T) {
	ctx := &Context{
		Transitive: map[rdf.Term]bool{},
		Symmetric:  map[rdf.Term]bool{rdf.IRI("ex:knows"): true},
		Inverses:   map[rdf.Term]rdf.Term{},
	}
	rules, _ := LoadDefault()
	engine := NewEngine(rules, ctx)

	base := []rdf.Triple{
		{Subject: rdf.IRI("ex:alice"), Predicate: rdf.IRI("ex:knows"), Object: rdf.IRI("ex:bob")},
	}

	result := engine.Derive(base)
	found := false
	for _, tr := range result.Derived {
		if tr.Subject.Equal(rdf.IRI("ex:bob")) &&
			tr.Predicate.Equal(rdf.IRI("ex:knows")) &&
			tr.Object.Equal(rdf.IRI("ex:alice")) {
			found = true
		}
	}
	if !found {
		t.Error("expected symmetric: bob knows alice")
	}
}

func TestEngineInverseOf(t *testing.T) {
	ctx := &Context{
		Transitive: map[rdf.Term]bool{},
		Symmetric:  map[rdf.Term]bool{},
		Inverses:   map[rdf.Term]rdf.Term{rdf.IRI("ex:parentOf"): rdf.IRI("ex:childOf")},
	}
	rules, _ := LoadDefault()
	engine := NewEngine(rules, ctx)

	base := []rdf.Triple{
		{Subject: rdf.IRI("ex:alice"), Predicate: rdf.IRI("ex:parentOf"), Object: rdf.IRI("ex:bob")},
	}

	result := engine.Derive(base)
	found := false
	for _, tr := range result.Derived {
		if tr.Subject.Equal(rdf.IRI("ex:bob")) &&
			tr.Predicate.Equal(rdf.IRI("ex:childOf")) &&
			tr.Object.Equal(rdf.IRI("ex:alice")) {
			found = true
		}
	}
	if !found {
		t.Error("expected inverse: bob childOf alice")
	}
}

func TestEngineDomain(t *testing.T) {
	rules, _ := LoadDefault()
	engine := NewEngine(rules, newTestCtx())

	base := []rdf.Triple{
		{Subject: rdf.IRI("ex:teaches"), Predicate: rdf.Domain, Object: rdf.IRI("ex:Teacher")},
		{Subject: rdf.IRI("ex:alice"), Predicate: rdf.IRI("ex:teaches"), Object: rdf.IRI("ex:bob")},
	}

	result := engine.Derive(base)
	found := false
	for _, tr := range result.Derived {
		if tr.Subject.Equal(rdf.IRI("ex:alice")) &&
			tr.Predicate.Equal(rdf.Type) &&
			tr.Object.Equal(rdf.IRI("ex:Teacher")) {
			found = true
		}
	}
	if !found {
		t.Error("expected domain inference: alice a Teacher")
	}
}

func TestEngineRange_IRIOnly(t *testing.T) {
	rules, _ := LoadDefault()
	engine := NewEngine(rules, newTestCtx())

	base := []rdf.Triple{
		{Subject: rdf.IRI("ex:teaches"), Predicate: rdf.Range, Object: rdf.IRI("ex:Student")},
		{Subject: rdf.IRI("ex:alice"), Predicate: rdf.IRI("ex:teaches"), Object: rdf.IRI("ex:bob")},
		{Subject: rdf.IRI("ex:alice"), Predicate: rdf.IRI("ex:teaches"), Object: rdf.Lit("123")},
	}

	result := engine.Derive(base)
	// bob 应该被推导为 Student
	bobFound := false
	for _, tr := range result.Derived {
		if tr.Subject.Equal(rdf.IRI("ex:bob")) &&
			tr.Predicate.Equal(rdf.Type) &&
			tr.Object.Equal(rdf.IRI("ex:Student")) {
			bobFound = true
		}
		// 字面量不应该被推导
		if tr.Subject.Equal(rdf.Lit("123")) {
			t.Error("literal should not get type inference")
		}
	}
	if !bobFound {
		t.Error("expected range inference: bob a Student")
	}
}

func TestEngineDisableAll(t *testing.T) {
	rules, _ := LoadDefault()
	for i := range rules {
		rules[i].Enabled = false
	}
	engine := NewEngine(rules, newTestCtx())

	base := []rdf.Triple{
		{Subject: rdf.IRI("ex:A"), Predicate: rdf.SubClassOf, Object: rdf.IRI("ex:B")},
		{Subject: rdf.IRI("ex:x"), Predicate: rdf.Type, Object: rdf.IRI("ex:A")},
	}

	result := engine.Derive(base)
	if len(result.Derived) != 0 {
		t.Errorf("expected 0 derived when all disabled, got %d", len(result.Derived))
	}
}

func TestEngineNoDeduplication(t *testing.T) {
	rules, _ := LoadDefault()
	engine := NewEngine(rules, newTestCtx())

	// 同一条三元组不应该出现在 derived 中
	base := []rdf.Triple{
		{Subject: rdf.IRI("ex:A"), Predicate: rdf.SubClassOf, Object: rdf.IRI("ex:B")},
		{Subject: rdf.IRI("ex:B"), Predicate: rdf.SubClassOf, Object: rdf.IRI("ex:C")},
		{Subject: rdf.IRI("ex:A"), Predicate: rdf.SubClassOf, Object: rdf.IRI("ex:C")}, // 已存在
	}

	result := engine.Derive(base)
	for _, tr := range result.Derived {
		if tr.Subject.Equal(rdf.IRI("ex:A")) &&
			tr.Predicate.Equal(rdf.SubClassOf) &&
			tr.Object.Equal(rdf.IRI("ex:C")) {
			t.Error("A subClassOf C should not be in derived (already in base)")
		}
	}
}

func TestEngineSortedOutput(t *testing.T) {
	rules, _ := LoadDefault()
	engine := NewEngine(rules, newTestCtx())

	base := []rdf.Triple{
		{Subject: rdf.IRI("ex:Z"), Predicate: rdf.SubClassOf, Object: rdf.IRI("ex:A")},
		{Subject: rdf.IRI("ex:A"), Predicate: rdf.SubClassOf, Object: rdf.IRI("ex:M")},
	}

	result := engine.Derive(base)
	for i := 1; i < len(result.Derived); i++ {
		if result.Derived[i-1].String() > result.Derived[i].String() {
			t.Error("derived triples should be sorted")
			break
		}
	}
}
