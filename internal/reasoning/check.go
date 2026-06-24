package reasoning

import (
	"sort"

	"github.com/walker/myonto/internal/rdf"
)

// Finding 是一致性检查发现的问题。
//
// 与 Derive 推导出的 []Triple 是不同种类的输出：
//   - 推导三元组是"隐式知识的显式化"，语义上成立，可安全物化。
//   - Finding 是"本体违反了自身声明的约束"，是错误报告，不可物化。
//
// 因此 Check 走独立输出通道，不经过 Derive 的不动点循环，也不被 materialize 写入。
type Finding struct {
	Severity string       // "error" | "warning"
	Rule     string       // 触发的检查规则，如 "owl:disjointWith"
	Subject  rdf.Term     // 违规个体
	Detail   string       // 人类可读说明
	Evidence []rdf.Triple // 支撑证据（用于定位）
}

// Check 跑一致性检查，返回所有发现的问题。
//
// 不修改本体，不推导三元组。检查基于"推导后的完整类型集"——
// 即先跑一次 Derive 拿到含继承关系的全部类型断言，再在此之上查约束违反。
// 这意味着 disjointWith 检测能捕获经由 subClassOf 继承得到的隐式冲突。
func (r *Reasoner) Check() []Finding {
	derived := r.Derive()
	findings := r.checkDisjointWith(derived)
	// 稳定排序：让输出可预测，便于测试和 CI diff。
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Rule != findings[j].Rule {
			return findings[i].Rule < findings[j].Rule
		}
		return findings[i].Subject.Value < findings[j].Subject.Value
	})
	return findings
}

// checkDisjointWith 检测 owl:disjointWith 约束违反。
//
// 规则：若 A disjointWith B，且存在个体 x 同时 a A 且 a B（含推导得到的类型），
// 则本体不一致，产生 error 级 Finding。
//
// disjointWith 本身是对称的（A disjointWith B ⟹ B disjointWith A），
// 但为避免重复报告，我们只对有序对的每一对查一次。
//
// 注意：disjointWith 声明本身是原始三元组，不会出现在 derived 里；
// 类型断言也分原始和推导两种。因此这里从 base+derived 的完整集扫描。
func (r *Reasoner) checkDisjointWith(derived []rdf.Triple) []Finding {
	// 完整三元组集 = 原始 + 推导。Check 必须在此之上判断。
	all := make([]rdf.Triple, 0, len(r.base)+len(derived))
	all = append(all, r.base...)
	all = append(all, derived...)

	// 收集所有 disjoint 声明，用有序对 key 去重。
	type pair struct{ a, b rdf.Term }
	pairKey := func(x, y rdf.Term) string {
		if x.Value > y.Value {
			x, y = y, x
		}
		return x.Value + "\x00" + y.Value
	}
	disjointPairs := map[string]pair{}
	for _, t := range all {
		if !t.Predicate.Equal(rdf.DisjointWith) {
			continue
		}
		k := pairKey(t.Subject, t.Object)
		disjointPairs[k] = pair{a: t.Subject, b: t.Object}
	}
	if len(disjointPairs) == 0 {
		return nil
	}

	// 建个体 → 类型集 索引（含推导得到的类型）。
	typesOf := map[rdf.Term]map[rdf.Term]bool{}
	for _, t := range all {
		if !t.Predicate.Equal(rdf.Type) {
			continue
		}
		// 跳过元类型（Class/owl:Class）。
		if t.Object.Equal(rdf.Class) || t.Object.Equal(rdf.OwlClass) {
			continue
		}
		if typesOf[t.Subject] == nil {
			typesOf[t.Subject] = map[rdf.Term]bool{}
		}
		typesOf[t.Subject][t.Object] = true
	}

	var findings []Finding
	for _, p := range disjointPairs {
		// 找同时拥有类型 a 和 b 的个体。
		for subj, types := range typesOf {
			if types[p.a] && types[p.b] {
				// 收集证据：该个体的两条 type 三元组 + disjointWith 声明。
				evidence := []rdf.Triple{
					{Subject: subj, Predicate: rdf.Type, Object: p.a},
					{Subject: subj, Predicate: rdf.Type, Object: p.b},
					{Subject: p.a, Predicate: rdf.DisjointWith, Object: p.b},
				}
				findings = append(findings, Finding{
					Severity: "error",
					Rule:     "owl:disjointWith",
					Subject:  subj,
					Detail:   subj.LocalName() + " 同时声明为 " + p.a.LocalName() + " 和 " + p.b.LocalName() + "，但这两个类互斥（disjointWith）",
					Evidence: evidence,
				})
			}
		}
	}
	return findings
}

// SeverityError / SeverityWarning 是 Finding 严重度常量，避免散写字符串。
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// ErrorCount 返回 findings 中 error 级问题的数量（供 --strict 门禁用）。
func ErrorCount(findings []Finding) int {
	n := 0
	for _, f := range findings {
		if f.Severity == SeverityError {
			n++
		}
	}
	return n
}

// WarningCount 返回 warning 级问题的数量。
func WarningCount(findings []Finding) int {
	n := 0
	for _, f := range findings {
		if f.Severity == SeverityWarning {
			n++
		}
	}
	return n
}
