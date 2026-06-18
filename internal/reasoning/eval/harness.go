// Package eval 为 internal/reasoning 提供可量化（P/R/F1）的评估框架。
//
// 设计目标：让"推理效果好不好"从主观判断变成 5 个硬数字。
// 参见 docs/reasoning-conformance.md 和仓库根 eval/ 的对拍脚本。
//
// 评估器与被评估的 reasoning 引擎解耦——本包只 import reasoning 跑推导，
// 然后把输出与 golden 期望集做集合运算，计算 precision/recall/F1。
// 这样新增规则、改不动点算法都不影响评估口径。
//
// 用例见 testdata/cases/<name>/{input.ttl, expect_derive.ttl, expect_not_derive.ttl, case.json}。
package eval

import (
	"fmt"

	"github.com/walker/myonto/internal/rdf"
	"github.com/walker/myonto/internal/reasoning"
)

// Result 是单个 case 的评估结果。
//
//   - TP (true positive)：期望推出且确实推出的三元组数。
//   - FP (false positive)：不该推出却推出的三元组（precision 损失，"推多了"）。
//   - FN (false negative)：该推出却没推出的三元组（recall 损失，"漏推了"）。
//   - Precision = TP / (TP + FP)；Recall = TP / (TP + FN)；F1 是调和平均。
//   - Missing：具体漏推的三元组（FN 的明细，便于定位）。
//   - Spurious：具体错推的三元组（FP 的明细）。
type Result struct {
	CaseName  string
	Rule      string
	TP, FP, FN int
	Precision float64
	Recall    float64
	F1        float64
	Missing   []rdf.Triple // 期望推出但没推出（FN）
	Spurious  []rdf.Triple // 不该推出却推出（FP）
}

// Pass 当且仅当 precision 和 recall 都为 1.0（无 FP 也无 FN）时返回 true。
// 这是 golden 回归的硬门槛：任何一条漏推或错推都判失败。
func (r Result) Pass() bool {
	return r.FP == 0 && r.FN == 0
}

// RunCase 在单个 case 上跑评估：用输入三元组构造 reasoner，Derive，
// 然后与期望集对比。
func RunCase(c Case) Result {
	r := reasoning.NewReasoner(c.Input)
	derived := r.Derive()
	return evaluate(c, derived)
}

// evaluate 把 derived 与期望集做集合运算，算 P/R/F1。
// 独立出来便于在不实际跑 reasoner 的情况下做单元测试（喂已知的 derived）。
func evaluate(c Case, derived []rdf.Triple) Result {
	derivedSet := indexTriples(derived)

	res := Result{CaseName: c.Name, Rule: c.Rule}

	// FN：期望推出但 derived 里没有。
	for _, want := range c.ExpectDerive {
		if _, ok := derivedSet[keyOf(want)]; ok {
			res.TP++
		} else {
			res.FN++
			res.Missing = append(res.Missing, want)
		}
	}
	// FP：不该推出却推出。
	negSet := indexTriples(c.ExpectNotDerive)
	for _, t := range derived {
		// 一条 derived 三元组若在负例集里 → 错推。
		if _, bad := negSet[keyOf(t)]; bad {
			res.FP++
			res.Spurious = append(res.Spurious, t)
		}
	}

	res.Precision, res.Recall, res.F1 = computeMetrics(res.TP, res.FP, res.FN)
	return res
}

// computeMetrics 由 TP/FP/FN 算 P/R/F1。
// TP+FP==0 时 precision 定义为 1（无错推即完美）；
// TP+FN==0 时 recall 定义为 1（无期望即无遗漏）。
// 这是评估"空 case"时的合理约定，避免 0/0 歧义。
func computeMetrics(tp, fp, fn int) (precision, recall, f1 float64) {
	if tp+fp == 0 {
		precision = 1.0
	} else {
		precision = float64(tp) / float64(tp+fp)
	}
	if tp+fn == 0 {
		recall = 1.0
	} else {
		recall = float64(tp) / float64(tp+fn)
	}
	// F1 调和平均；P+R==0 时定义为 0。
	if precision+recall == 0 {
		f1 = 0.0
	} else {
		f1 = 2 * precision * recall / (precision + recall)
	}
	return
}

func indexTriples(ts []rdf.Triple) map[string]rdf.Triple {
	m := make(map[string]rdf.Triple, len(ts))
	for _, t := range ts {
		m[keyOf(t)] = t
	}
	return m
}

// keyOf 用三元组的规范字符串作去重 key。与 reasoning 内部的 tripleKey 一致。
func keyOf(t rdf.Triple) string { return t.String() }

// String 给出人类可读的单 case 结果摘要（用于失败诊断）。
func (r Result) String() string {
	status := "✓ PASS"
	if !r.Pass() {
		status = "✗ FAIL"
	}
	s := fmt.Sprintf("[%s] %s (rule=%s) P=%.2f R=%.2f F1=%.2f TP=%d FP=%d FN=%d",
		status, r.CaseName, r.Rule, r.Precision, r.Recall, r.F1, r.TP, r.FP, r.FN)
	for _, m := range r.Missing {
		s += fmt.Sprintf("\n    漏推(FN): %s", m)
	}
	for _, sp := range r.Spurious {
		s += fmt.Sprintf("\n    错推(FP): %s", sp)
	}
	return s
}
