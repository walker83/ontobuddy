package reasoning

import (
	"testing"

	"github.com/walker/myonto/internal/rdf"
)

// 这个文件专门放"不该推出的东西"——负例测试。
//
// 现有 reasoning_test.go 全是正例（hasTriple(...)），无法捕获"推多了"这类回归：
// 例如哪天有人把 rdf:type 也纳入子属性继承，正例不会变红，
// 但我们会开始往本体里写一堆荒谬的推论。
//
// 负例断言用 !hasTriple(...) 显式钉死边界，是评估"可靠性"的最低门槛。
// 更系统的 P/R/F1 评估见 internal/reasoning/eval/。

// notHasTriple 判断输出是否*不含*某条三元组（hasTriple 的负例版）。
func notHasTriple(ts []rdf.Triple, unwanted rdf.Triple) bool { return !hasTriple(ts, unwanted) }

// --- 规则 1：subClassOf 传递 ---

// TestNoSpuriousSelfLoop 验证等价环 A⊑B, B⊑A 不应推出自环 A⊑A。
// reasoning.go:144 有专门的 a.Equal(c) 过滤，但此前无测试覆盖。
func TestNoSpuriousSelfLoop(t *testing.T) {
	a, b := makeIRI("A"), makeIRI("B")
	r := NewReasoner([]rdf.Triple{
		{Subject: a, Predicate: rdf.SubClassOf, Object: b},
		{Subject: b, Predicate: rdf.SubClassOf, Object: a},
	})
	got := r.Derive()
	selfLoop := rdf.Triple{Subject: a, Predicate: rdf.SubClassOf, Object: a}
	if hasTriple(got, selfLoop) {
		t.Errorf("等价环不应推出自环 A⊑A，实际: %v", got)
	}
	selfLoopB := rdf.Triple{Subject: b, Predicate: rdf.SubClassOf, Object: b}
	if hasTriple(got, selfLoopB) {
		t.Errorf("等价环不应推出自环 B⊑B，实际: %v", got)
	}
}

// --- 规则 2：类型继承 ---

// TestNoTypePropagationToClassMeta 验证类型继承只对"个体→类"生效，
// 不应把类的元关系当成个体实例化（如 A a rdfs:Class 不应推出 A a owl:Class）。
// 同时验证 type 继承不会越过到 Class/OwlClass 这类元类。
func TestNoTypePropagationToClassMeta(t *testing.T) {
	a, b := makeIRI("A"), makeIRI("B")
	x := makeIRI("x")
	r := NewReasoner([]rdf.Triple{
		{Subject: a, Predicate: rdf.SubClassOf, Object: b},
		{Subject: b, Predicate: rdf.Type, Object: rdf.Class}, // B 是元类，非普通父类
		{Subject: x, Predicate: rdf.Type, Object: a},
	})
	got := r.Derive()
	// x a A ⟹ x a B（正常类型继承，应该推）
	if !hasTriple(got, rdf.Triple{Subject: x, Predicate: rdf.Type, Object: b}) {
		t.Errorf("应推出 x a B，实际: %v", got)
	}
	// 但 B a rdfs:Class 不应被当成"x 的父类的父类"传播给 x。
	// reasoning.go:172-174 明确排除 Class/OwlClass 的元数据。
	if hasTriple(got, rdf.Triple{Subject: x, Predicate: rdf.Type, Object: rdf.Class}) {
		t.Errorf("元类 rdfs:Class 不应被传播给个体，实际: %v", got)
	}
}

// --- 规则 5：传递属性 ---

// TestTransitiveDoesNotApplyToNonTransitivePred 验证未声明 Transitive 的谓词不做闭包。
// 例如普通的 :knows（未声明 Transitive）不应推出 a knows c。
func TestTransitiveDoesNotApplyToNonTransitivePred(t *testing.T) {
	a, b, c := makeIRI("a"), makeIRI("b"), makeIRI("c")
	foo := makeIRI("foo") // 任意谓词，未声明任何 owl 类型
	r := NewReasoner([]rdf.Triple{
		{Subject: a, Predicate: foo, Object: b},
		{Subject: b, Predicate: foo, Object: c},
	})
	got := r.Derive()
	if hasTriple(got, rdf.Triple{Subject: a, Predicate: foo, Object: c}) {
		t.Errorf("非 Transitive 谓词不应做传递闭包，实际推: %v", got)
	}
}

// --- 规则 6：对称属性 ---

// TestSymmetricDoesNotInvertArbitraryPred 验证未声明 Symmetric 的谓词不翻转方向。
func TestSymmetricDoesNotInvertArbitraryPred(t *testing.T) {
	a, b := makeIRI("a"), makeIRI("b")
	parentOf := makeIRI("parentOf") // 非对称：a parentOf b 不应推出 b parentOf a
	r := NewReasoner([]rdf.Triple{
		{Subject: a, Predicate: parentOf, Object: b},
	})
	got := r.Derive()
	if hasTriple(got, rdf.Triple{Subject: b, Predicate: parentOf, Object: a}) {
		t.Errorf("非 Symmetric 谓词不应翻转，实际推: %v", got)
	}
}

// --- 规则 4：属性继承 ---

// TestSubPropertyInheritanceSkipsMetaPredicates 验证 rdf:type/rdfs:label
// 等元谓词即使出现在 subPropertyOf 体系里也不被继承。
// reasoning.go:227-231 有专门的元谓词跳过清单。
func TestSubPropertyInheritanceSkipsMetaPredicates(t *testing.T) {
	a, b := makeIRI("a"), makeIRI("b")
	specialType := makeIRI("specialType") // 子属性
	// 声明 specialType ⊑ rdf:type（一个故意刁钻的构造）
	// 然后 a specialType b —— 按规则 4 应推出 a rdf:type b
	// 但若有人反过来声明 rdf:type ⊑ something，something 不应继承 type 的实例。
	// 这里我们构造反向：rdf:type ⊑ specialType，断言现有的 a rdf:type A 三元组
	// *应该* 推出 a specialType A？不——rdf.Type 在跳过清单里，所以不应推。
	r := NewReasoner([]rdf.Triple{
		{Subject: rdf.Type, Predicate: rdf.SubPropertyOf, Object: specialType},
		{Subject: a, Predicate: rdf.Type, Object: b},
	})
	got := r.Derive()
	if hasTriple(got, rdf.Triple{Subject: a, Predicate: specialType, Object: b}) {
		t.Errorf("rdf:type 应在元谓词跳过清单里，不应被子属性继承，实际推: %v", got)
	}
}

// --- 规则 3：subPropertyOf 传递（此前完全缺测） ---

// TestSubPropertyOfTransitive 补齐规则 3 的独立测试：
// P⊑Q, Q⊑R ⟹ P⊑R。当前测试文件只间接通过规则 4 触及。
func TestSubPropertyOfTransitive(t *testing.T) {
	p, q, r := makeIRI("P"), makeIRI("Q"), makeIRI("R")
	reas := NewReasoner([]rdf.Triple{
		{Subject: p, Predicate: rdf.SubPropertyOf, Object: q},
		{Subject: q, Predicate: rdf.SubPropertyOf, Object: r},
	})
	got := reas.Derive()
	if !hasTriple(got, rdf.Triple{Subject: p, Predicate: rdf.SubPropertyOf, Object: r}) {
		t.Errorf("应推出 P⊑R（subPropertyOf 传递），实际: %v", got)
	}
}

// --- 不动点 / 终止性 ---

// TestTerminatesOnCycle 验证循环类层级 A⊑B, B⊑A 不会让不动点循环死掉。
// 用一个 timeout 风格的断言：如果 1 秒内没返回就算失败（不动点已坏）。
// 这里靠 testing 本身的超时不够，所以我们直接断言 Derive() 返回且非 panic。
// （真正的死循环会被 Go test 的默认 -timeout 10min 抓到，CI 里会超时红。）
func TestTerminatesOnCycle(t *testing.T) {
	a, b, c := makeIRI("A"), makeIRI("B"), makeIRI("C")
	// A⊑B, B⊑A, B⊑C, C⊑A —— 三角环
	r := NewReasoner([]rdf.Triple{
		{Subject: a, Predicate: rdf.SubClassOf, Object: b},
		{Subject: b, Predicate: rdf.SubClassOf, Object: a},
		{Subject: b, Predicate: rdf.SubClassOf, Object: c},
		{Subject: c, Predicate: rdf.SubClassOf, Object: a},
	})
	got := r.Derive()
	// 不应含任何自环。
	for _, tr := range got {
		if tr.Subject.Equal(tr.Object) {
			t.Errorf("循环层级中不应推出自环 %s", tr)
		}
	}
}

// TestSaturatesExactly 验证一条简单链上推导的条数恰好——既不少推也不多推。
// 输入：A⊑B⊑C⊑D，x a A。
// 期望推导（不含原始）：
//   subClassOf 传递：A⊑C, A⊑D, B⊑D              → 3 条
//   type 继承：x a B, x a C, x a D              → 3 条
// 共 6 条。任何偏离（5 或 7）都说明推理机多推或少推了。
func TestSaturatesExactly(t *testing.T) {
	a, b, c, d := makeIRI("A"), makeIRI("B"), makeIRI("C"), makeIRI("D")
	x := makeIRI("x")
	r := NewReasoner([]rdf.Triple{
		{Subject: a, Predicate: rdf.SubClassOf, Object: b},
		{Subject: b, Predicate: rdf.SubClassOf, Object: c},
		{Subject: c, Predicate: rdf.SubClassOf, Object: d},
		{Subject: x, Predicate: rdf.Type, Object: a},
	})
	got := r.Derive()
	const want = 6
	if len(got) != want {
		t.Errorf("应恰好推出 %d 条，实际 %d 条:\n", want, len(got))
		for _, tr := range got {
			t.Logf("  + %s", tr)
		}
	}
	// 逐条核对，防计数巧合。
	wantSet := []rdf.Triple{
		{Subject: a, Predicate: rdf.SubClassOf, Object: c},
		{Subject: a, Predicate: rdf.SubClassOf, Object: d},
		{Subject: b, Predicate: rdf.SubClassOf, Object: d},
		{Subject: x, Predicate: rdf.Type, Object: b},
		{Subject: x, Predicate: rdf.Type, Object: c},
		{Subject: x, Predicate: rdf.Type, Object: d},
	}
	for _, w := range wantSet {
		if !hasTriple(got, w) {
			t.Errorf("缺少期望三元组: %s", w)
		}
	}
}

// _ 防止 notHasTriple 在未来被误删时编译失败提示——保留辅助函数引用。
var _ = notHasTriple
