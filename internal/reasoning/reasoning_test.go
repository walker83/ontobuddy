package reasoning

import (
	"testing"

	"github.com/walker/myonto/internal/rdf"
)

// makeIRI 简化测试中构造 IRI Term 的样板。
func makeIRI(s string) rdf.Term { return rdf.IRI(s) }

// hasTriple 判断输出是否包含某条三元组。
func hasTriple(ts []rdf.Triple, want rdf.Triple) bool {
	for _, t := range ts {
		if t.Equal(want) {
			return true
		}
	}
	return false
}

// TestSubClassOfTransitive 验证 subClassOf 传递闭包：A ⊑ B, B ⊑ C ⟹ A ⊑ C
func TestSubClassOfTransitive(t *testing.T) {
	a, b, c := makeIRI("A"), makeIRI("B"), makeIRI("C")
	r := NewReasoner([]rdf.Triple{
		{Subject: a, Predicate: rdf.SubClassOf, Object: b},
		{Subject: b, Predicate: rdf.SubClassOf, Object: c},
	})
	got := r.Derive()
	want := rdf.Triple{Subject: a, Predicate: rdf.SubClassOf, Object: c}
	if !hasTriple(got, want) {
		t.Errorf("应推出 A ⊑ C，实际: %v", got)
	}
}

// TestTypeInheritance 验证 x a A, A ⊑ B ⟹ x a B
func TestTypeInheritance(t *testing.T) {
	a, b, x := makeIRI("A"), makeIRI("B"), makeIRI("x")
	r := NewReasoner([]rdf.Triple{
		{Subject: a, Predicate: rdf.SubClassOf, Object: b},
		{Subject: x, Predicate: rdf.Type, Object: a},
	})
	got := r.Derive()
	want := rdf.Triple{Subject: x, Predicate: rdf.Type, Object: b}
	if !hasTriple(got, want) {
		t.Errorf("应推出 x a B，实际: %v", got)
	}
}

// TestTypeInheritanceChain 验证多级继承：Scientist ⊑ Person ⊑ Agent, newton a Scientist ⟹ newton a Person / Agent
func TestTypeInheritanceChain(t *testing.T) {
	scientist, person, agent, newton :=
		makeIRI("Scientist"), makeIRI("Person"), makeIRI("Agent"), makeIRI("newton")
	r := NewReasoner([]rdf.Triple{
		{Subject: scientist, Predicate: rdf.SubClassOf, Object: person},
		{Subject: person, Predicate: rdf.SubClassOf, Object: agent},
		{Subject: newton, Predicate: rdf.Type, Object: scientist},
	})
	got := r.Derive()

	if !hasTriple(got, rdf.Triple{Subject: scientist, Predicate: rdf.SubClassOf, Object: agent}) {
		t.Error("应推出 Scientist ⊑ Agent")
	}
	if !hasTriple(got, rdf.Triple{Subject: newton, Predicate: rdf.Type, Object: person}) {
		t.Error("应推出 newton a Person")
	}
	if !hasTriple(got, rdf.Triple{Subject: newton, Predicate: rdf.Type, Object: agent}) {
		t.Error("应推出 newton a Agent")
	}
}

// TestTransitiveProperty 验证传递属性：a P b, b P c, P:Transitive ⟹ a P c
func TestTransitiveProperty(t *testing.T) {
	a, b, c := makeIRI("a"), makeIRI("b"), makeIRI("c")
	partOf := makeIRI("partOf")
	r := NewReasoner([]rdf.Triple{
		{Subject: partOf, Predicate: rdf.Type, Object: rdf.TransitiveProperty},
		{Subject: a, Predicate: partOf, Object: b},
		{Subject: b, Predicate: partOf, Object: c},
	})
	got := r.Derive()
	if !hasTriple(got, rdf.Triple{Subject: a, Predicate: partOf, Object: c}) {
		t.Errorf("应推出 a partOf c，实际: %v", got)
	}
}

// TestSymmetricProperty 验证对称属性：a P b, P:Symmetric ⟹ b P a
func TestSymmetricProperty(t *testing.T) {
	a, b := makeIRI("a"), makeIRI("b")
	knows := makeIRI("knows")
	r := NewReasoner([]rdf.Triple{
		{Subject: knows, Predicate: rdf.Type, Object: rdf.SymmetricProperty},
		{Subject: a, Predicate: knows, Object: b},
	})
	got := r.Derive()
	if !hasTriple(got, rdf.Triple{Subject: b, Predicate: knows, Object: a}) {
		t.Errorf("应推出 b knows a，实际: %v", got)
	}
}

// TestInverseOf 验证逆关系：a P b, P inverseOf Q ⟹ b Q a
func TestInverseOf(t *testing.T) {
	a, b := makeIRI("a"), makeIRI("b")
	teaches, taughtBy := makeIRI("teaches"), makeIRI("taughtBy")
	r := NewReasoner([]rdf.Triple{
		{Subject: teaches, Predicate: rdf.InverseOf, Object: taughtBy},
		{Subject: a, Predicate: teaches, Object: b},
	})
	got := r.Derive()
	if !hasTriple(got, rdf.Triple{Subject: b, Predicate: taughtBy, Object: a}) {
		t.Errorf("应推出 b taughtBy a，实际: %v", got)
	}
}

// TestSubPropertyOfInheritance 验证子属性继承：a P b, P ⊑ Q ⟹ a Q b
func TestSubPropertyOfInheritance(t *testing.T) {
	a, b := makeIRI("a"), makeIRI("b")
	fatherOf, parentOf := makeIRI("fatherOf"), makeIRI("parentOf")
	r := NewReasoner([]rdf.Triple{
		{Subject: fatherOf, Predicate: rdf.SubPropertyOf, Object: parentOf},
		{Subject: a, Predicate: fatherOf, Object: b},
	})
	got := r.Derive()
	if !hasTriple(got, rdf.Triple{Subject: a, Predicate: parentOf, Object: b}) {
		t.Errorf("应推出 a parentOf b，实际: %v", got)
	}
}

// TestNoDerivationWhenNoRuleApplies 确保不产生多余推论。
func TestNoDerivationWhenNoRuleApplies(t *testing.T) {
	a, b := makeIRI("a"), makeIRI("b")
	foo := makeIRI("foo")
	r := NewReasoner([]rdf.Triple{
		{Subject: a, Predicate: foo, Object: b},
	})
	got := r.Derive()
	if len(got) != 0 {
		t.Errorf("不应有推论，得到: %v", got)
	}
}

// TestOriginalsNotInOutput 确保原始三元组不会出现在推导结果中。
func TestOriginalsNotInOutput(t *testing.T) {
	a, b, c := makeIRI("a"), makeIRI("b"), makeIRI("c")
	r := NewReasoner([]rdf.Triple{
		{Subject: a, Predicate: rdf.SubClassOf, Object: b},
	})
	got := r.Derive()
	// 原始 a⊑b 不应在输出里。
	if hasTriple(got, rdf.Triple{Subject: a, Predicate: rdf.SubClassOf, Object: b}) {
		t.Error("原始三元组不应出现在推导结果中")
	}
	_ = c
}
