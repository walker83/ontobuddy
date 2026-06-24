package reasoning

import (
	"testing"

	"github.com/walker/myonto/internal/rdf"
)

// TestCheck_DisjointWith_Violation 验证直接冲突检测：
// Cat disjointWith Dog, felix a Cat, a Dog ⟹ 1 个 error finding。
func TestCheck_DisjointWith_Violation(t *testing.T) {
	cat, dog, felix := makeIRI("Cat"), makeIRI("Dog"), makeIRI("felix")
	r := NewReasoner([]rdf.Triple{
		{Subject: cat, Predicate: rdf.DisjointWith, Object: dog},
		{Subject: felix, Predicate: rdf.Type, Object: cat},
		{Subject: felix, Predicate: rdf.Type, Object: dog},
	})
	findings := r.Check()
	if len(findings) != 1 {
		t.Fatalf("应发现 1 个冲突，实际: %d (%v)", len(findings), findings)
	}
	if findings[0].Severity != SeverityError {
		t.Errorf("严重度应为 error，实际: %s", findings[0].Severity)
	}
	if findings[0].Rule != "owl:disjointWith" {
		t.Errorf("规则应为 owl:disjointWith，实际: %s", findings[0].Rule)
	}
	if !findings[0].Subject.Equal(felix) {
		t.Errorf("违规个体应为 felix，实际: %s", findings[0].Subject)
	}
	if ErrorCount(findings) != 1 || WarningCount(findings) != 0 {
		t.Errorf("error=1 warning=0，实际 error=%d warning=%d", ErrorCount(findings), WarningCount(findings))
	}
}

// TestCheck_DisjointWith_NoViolation 验证无冲突时不报问题。
func TestCheck_DisjointWith_NoViolation(t *testing.T) {
	cat, dog, felix, rex := makeIRI("Cat"), makeIRI("Dog"), makeIRI("felix"), makeIRI("rex")
	r := NewReasoner([]rdf.Triple{
		{Subject: cat, Predicate: rdf.DisjointWith, Object: dog},
		{Subject: felix, Predicate: rdf.Type, Object: cat},
		{Subject: rex, Predicate: rdf.Type, Object: dog},
	})
	findings := r.Check()
	if len(findings) != 0 {
		t.Errorf("无冲突时不应有 finding，实际: %v", findings)
	}
}

// TestCheck_DisjointWith_InheritedConflict 验证经类型继承得到的隐式冲突：
// Cat disjointWith Dog, felix a Kitten, Kitten ⊑ Cat ⟹ felix 隐式 a Cat，
// 若 felix 同时 a Dog，应被检测到（这是 Check 基于 derived 类型集的核心价值）。
func TestCheck_DisjointWith_InheritedConflict(t *testing.T) {
	cat, dog, kitten, felix :=
		makeIRI("Cat"), makeIRI("Dog"), makeIRI("Kitten"), makeIRI("felix")
	r := NewReasoner([]rdf.Triple{
		{Subject: cat, Predicate: rdf.DisjointWith, Object: dog},
		{Subject: kitten, Predicate: rdf.SubClassOf, Object: cat},
		// felix 显式是 Kitten（继承得 Cat）和 Dog
		{Subject: felix, Predicate: rdf.Type, Object: kitten},
		{Subject: felix, Predicate: rdf.Type, Object: dog},
	})
	findings := r.Check()
	if len(findings) != 1 {
		t.Fatalf("应检测到经继承的隐式冲突，实际 findings=%d (%v)", len(findings), findings)
	}
	if !findings[0].Subject.Equal(felix) {
		t.Errorf("违规个体应为 felix，实际: %s", findings[0].Subject)
	}
}

// TestCheck_DisjointWith_SymmetricDedup 验证 disjointWith 的对称声明不重复报告。
// 写两次（Cat disjointWith Dog 和 Dog disjointWith Cat）应只报一个 finding。
func TestCheck_DisjointWith_SymmetricDedup(t *testing.T) {
	cat, dog, felix := makeIRI("Cat"), makeIRI("Dog"), makeIRI("felix")
	r := NewReasoner([]rdf.Triple{
		{Subject: cat, Predicate: rdf.DisjointWith, Object: dog},
		{Subject: dog, Predicate: rdf.DisjointWith, Object: cat}, // 对称声明
		{Subject: felix, Predicate: rdf.Type, Object: cat},
		{Subject: felix, Predicate: rdf.Type, Object: dog},
	})
	findings := r.Check()
	if len(findings) != 1 {
		t.Errorf("对称声明应去重，应只 1 个 finding，实际: %d", len(findings))
	}
}

// TestCheck_NoDisjointWith_NoFindings 验证无 disjointWith 声明时不产生 finding。
func TestCheck_NoDisjointWith_NoFindings(t *testing.T) {
	cat, felix := makeIRI("Cat"), makeIRI("felix")
	r := NewReasoner([]rdf.Triple{
		{Subject: felix, Predicate: rdf.Type, Object: cat},
	})
	findings := r.Check()
	if len(findings) != 0 {
		t.Errorf("无 disjointWith 声明不应有 finding，实际: %v", findings)
	}
}
