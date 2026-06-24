package eval

import (
	"encoding/json"
	"fmt"

	"github.com/walker/myonto/internal/rdf"
	"github.com/walker/myonto/internal/reasoning"
)

// ExpectedFinding 是 case.json / expect_findings.json 里期望的检查发现。
//
// 匹配规则：Rule + Subject 必须精确匹配实际 Finding；
// Severity 默认 "error"（与 disjointWith 检测一致），可显式指定。
type ExpectedFinding struct {
	Severity string `json:"severity"` // "error" | "warning"，缺省视为 "error"
	Rule     string `json:"rule"`     // 如 "owl:disjointWith"
	Subject  string `json:"subject"`  // 实体的 local name 或完整 IRI
}

// CheckResult 是单个 case 的检查评估结果。
//
//   - Hit：期望发现且确实发现的 Finding 数。
//   - Missed：期望发现却没发现（应报未报）。
//   - Extra：未期望却发现的（多报）。
type CheckResult struct {
	CaseName string
	Hit      int
	Missed   []ExpectedFinding
	Extra    []reasoning.Finding
}

// Pass 当且仅当无 Missed 也无 Extra 时通过。
func (r CheckResult) Pass() bool {
	return len(r.Missed) == 0 && len(r.Extra) == 0
}

// String 给出人类可读摘要。
func (r CheckResult) String() string {
	status := "✓ PASS"
	if !r.Pass() {
		status = "✗ FAIL"
	}
	s := fmt.Sprintf("[%s] %s (check) hit=%d missed=%d extra=%d",
		status, r.CaseName, r.Hit, len(r.Missed), len(r.Extra))
	for _, m := range r.Missed {
		s += fmt.Sprintf("\n    应报未报: %s %s %s", m.Severity, m.Rule, m.Subject)
	}
	for _, e := range r.Extra {
		s += fmt.Sprintf("\n    多报: %s %s %s", e.Severity, e.Rule, e.Subject.LocalName())
	}
	return s
}

// RunCheckCase 在单个 case 上跑一致性检查评估。
//
// 用输入三元组构造 reasoner，调 Check() 拿到实际 findings，
// 与期望集按 Rule+Subject 匹配。
// 仅对声明了 ExpectFindings 的 case 有意义。
func RunCheckCase(c Case) CheckResult {
	r := reasoning.NewReasoner(c.Input)
	actual := r.Check()

	return evaluateCheck(c, actual)
}

// evaluateCheck 把实际 findings 与期望集对比，独立出来便于单测。
func evaluateCheck(c Case, actual []reasoning.Finding) CheckResult {
	res := CheckResult{CaseName: c.Name}

	// 实际 findings 按 "rule|subjectIRI" 索引，便于查 extra。
	actualByKey := map[string]reasoning.Finding{}
	for _, f := range actual {
		key := f.Rule + "|" + f.Subject.Value
		actualByKey[key] = f
	}

	// 逐个期望 finding 匹配。
	for _, want := range c.ExpectFindings {
		sev := want.Severity
		if sev == "" {
			sev = reasoning.SeverityError
		}
		// 解析 subject：支持 local name（相对 case 命名空间）或完整 IRI。
		// 测试 case 普遍用 ex: 前缀，这里做宽松匹配——subject 字符串作为 IRI 后缀匹配。
		matched := false
		for key, f := range actualByKey {
			if f.Rule == want.Rule && subjectMatches(f.Subject, want.Subject) && f.Severity == sev {
				res.Hit++
				delete(actualByKey, key) // 消费掉，避免重复计入
				matched = true
				break
			}
		}
		if !matched {
			res.Missed = append(res.Missed, want)
		}
	}

	// 剩余的 actual 即 extra（多报的）。
	for _, f := range actualByKey {
		res.Extra = append(res.Extra, f)
	}
	return res
}

// subjectMatches 判断实际 finding 的 subject 是否匹配期望字符串。
// 期望可以是完整 IRI 或 local name（取 IRI 最后一段匹配）。
func subjectMatches(actual rdf.Term, want string) bool {
	if actual.Value == want {
		return true
	}
	return actual.LocalName() == want
}

// loadExpectFindings 从 case.json 的 expect_findings 字段解析期望发现。
// 嵌入在 case.json 里而非单独文件，保持 case 元数据集中。
func loadExpectFindings(metaBytes []byte) []ExpectedFinding {
	if len(metaBytes) == 0 {
		return nil
	}
	var raw struct {
		ExpectFindings []ExpectedFinding `json:"expect_findings"`
	}
	if err := json.Unmarshal(metaBytes, &raw); err != nil {
		return nil
	}
	return raw.ExpectFindings
}
