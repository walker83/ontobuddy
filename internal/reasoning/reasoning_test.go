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

// TestDomainRule 验证 rdfs:domain：x P y, P domain C ⟹ x a C。
// 同时验证 domain 推出的类型会喂回 typeInheritanceRule（newton a Person ⊑ Agent ⟹ newton a Agent）。
func TestDomainRule(t *testing.T) {
	wrote, person, agent, newton, principia :=
		makeIRI("wrote"), makeIRI("Person"), makeIRI("Agent"), makeIRI("newton"), makeIRI("principia")
	r := NewReasoner([]rdf.Triple{
		{Subject: wrote, Predicate: rdf.Domain, Object: person},
		{Subject: person, Predicate: rdf.SubClassOf, Object: agent},
		{Subject: newton, Predicate: wrote, Object: principia},
	})
	got := r.Derive()
	// domain 直接结论
	if !hasTriple(got, rdf.Triple{Subject: newton, Predicate: rdf.Type, Object: person}) {
		t.Errorf("应推出 newton a Person，实际: %v", got)
	}
	// 与 typeInheritance 协同：Person ⊑ Agent ⟹ newton a Agent
	if !hasTriple(got, rdf.Triple{Subject: newton, Predicate: rdf.Type, Object: agent}) {
		t.Errorf("应推出 newton a Agent（domain + 类型继承），实际: %v", got)
	}
}

// TestRangeRule 验证 rdfs:range：x P y, P range C ⟹ y a C（y 为 IRI）。
func TestRangeRule(t *testing.T) {
	wrote, work, newton, principia :=
		makeIRI("wrote"), makeIRI("Work"), makeIRI("newton"), makeIRI("principia")
	r := NewReasoner([]rdf.Triple{
		{Subject: wrote, Predicate: rdf.Range, Object: work},
		{Subject: newton, Predicate: wrote, Object: principia},
	})
	got := r.Derive()
	if !hasTriple(got, rdf.Triple{Subject: principia, Predicate: rdf.Type, Object: work}) {
		t.Errorf("应推出 principia a Work，实际: %v", got)
	}
}

// TestRangeRule_LiteralSkipped 是 soundness 守卫：range 不给字面量宾语加类型。
// 字面量无类型身份，给它加 rdf:type 语义错误。TTL 不允许字面量做主语，
// 所以这个负例无法写成 eval golden case，只能在此单测覆盖。
func TestRangeRule_LiteralSkipped(t *testing.T) {
	wrote, place, newton :=
		makeIRI("wrote"), makeIRI("Place"), makeIRI("newton")
	r := NewReasoner([]rdf.Triple{
		{Subject: wrote, Predicate: rdf.Range, Object: place},
		// 宾语是字面量 "Woolsthorpe"
		{Subject: newton, Predicate: wrote, Object: rdf.Lit("Woolsthorpe")},
	})
	got := r.Derive()
	for _, gt := range got {
		// 任何以字面量为主语的三元组都不应出现
		if gt.Subject.Kind == rdf.KindLiteral {
			t.Errorf("不应给字面量加类型，但推出: %v", gt)
		}
	}
}

// TestDomainRange_MetaPredicateSkipped 验证 domain/range 不对元谓词三元组生效。
// 例如 newton a Person 不应触发 domain（type 不是用户数据谓词）。
func TestDomainRange_MetaPredicateSkipped(t *testing.T) {
	wrote, person, newton := makeIRI("wrote"), makeIRI("Person"), makeIRI("newton")
	// 给 wrote 声明 domain，但不给 type/label 等元谓词做任何 domain 推导
	r := NewReasoner([]rdf.Triple{
		{Subject: wrote, Predicate: rdf.Domain, Object: person},
		// 这条三元组的谓词是 rdf:type（元谓词），不应触发 domain
		{Subject: newton, Predicate: rdf.Type, Object: person},
	})
	got := r.Derive()
	// newton a person 是原始三元组，不应被重复推出
	// 也不应有其他污染。关键：不应因为 type 出现而错误推导。
	for _, gt := range got {
		if gt.Predicate.Equal(rdf.Type) && gt.Subject.Equal(newton) {
			t.Errorf("元谓词 type 不应触发 domain 推导，但推出: %v", gt)
		}
	}
}
