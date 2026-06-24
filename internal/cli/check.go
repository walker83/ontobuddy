package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/walker/myonto/internal/reasoning"
	"github.com/walker/myonto/internal/store"
)

// CheckCmd 跑一致性检查，报告本体中违反自身声明的约束。
//
// 与 reason 不同：reason 推导"隐含成立"的三元组（可物化）；
// check 报告"不应存在"的矛盾（不可物化，是错误报告）。
// 当前支持 owl:disjointWith 违规检测。
//
// --strict 让任何 error 级 Finding 导致 exit 1，便于 CI 门禁。
type CheckCmd struct {
	Strict bool `help:"有任何 error 级问题时退出码 1（CI 用）"`
}

// checkResult 是 --json 模式的输出结构。
type checkResult struct {
	Findings []checkFinding `json:"findings"`
	Errors   int            `json:"errors"`
	Warnings int            `json:"warnings"`
}

// checkFinding 是单条 Finding 的 JSON 表示。
type checkFinding struct {
	Severity string         `json:"severity"`
	Rule     string         `json:"rule"`
	Subject  string         `json:"subject"`
	Detail   string         `json:"detail"`
	Evidence []jsonTriple   `json:"evidence,omitempty"`
}

// Run 执行一致性检查。
func (c *CheckCmd) Run() error {
	s, _, err := openStore()
	if err != nil {
		return err
	}
	findings := runCheck(s)
	return checkRender(c, findings)
}

// runCheck 在 store 上跑一致性检查，返回 findings。
// 抽出来便于测试（不依赖文件 IO）。
func runCheck(s *store.Store) []reasoning.Finding {
	r := reasoning.NewReasoner(s.Triples())
	return r.Check()
}

// checkRender 统一处理 --json / 人类可读输出。
func checkRender(c *CheckCmd, findings []reasoning.Finding) error {
	errors := reasoning.ErrorCount(findings)
	warnings := reasoning.WarningCount(findings)

	if IsJSON() {
		out := checkResult{
			Errors:   errors,
			Warnings: warnings,
			Findings: make([]checkFinding, 0, len(findings)),
		}
		for _, f := range findings {
			cf := checkFinding{
				Severity: f.Severity,
				Rule:     f.Rule,
				Subject:  f.Subject.Value,
				Detail:   f.Detail,
			}
			for _, e := range f.Evidence {
				cf.Evidence = append(cf.Evidence, jsonTriple{
					Subject:   e.Subject.Value,
					Predicate: e.Predicate.Value,
					Object:    termToJSON(e.Object),
				})
			}
			out.Findings = append(out.Findings, cf)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		// --strict 门禁：JSON 模式也尊重
		if c.Strict && errors > 0 {
			exitWith(1)
		}
		return nil
	}

	// 人类可读路径
	if len(findings) == 0 {
		fmt.Fprintln(os.Stdout, "✓ 一致性检查通过：未发现约束违反")
		return nil
	}

	// 按严重度分组展示。
	errs := filterBySeverity(findings, reasoning.SeverityError)
	warns := filterBySeverity(findings, reasoning.SeverityWarning)

	if len(errs) > 0 {
		fmt.Fprintf(os.Stdout, "✗ 发现 %d 个错误（本体不一致）：\n\n", len(errs))
		for _, f := range errs {
			renderFinding(f)
		}
	}
	if len(warns) > 0 {
		fmt.Fprintf(os.Stdout, "\n⚠ 发现 %d 个警告：\n\n", len(warns))
		for _, f := range warns {
			renderFinding(f)
		}
	}

	fmt.Fprintf(os.Stdout, "\n汇总：%d 错误，%d 警告\n", errors, warnings)
	if c.Strict && errors > 0 {
		fmt.Fprintln(os.Stdout, "（--strict 模式：有错误，退出码 1）")
		exitWith(1)
	}
	return nil
}

// renderFinding 打印单条 Finding 的人类可读形式。
func renderFinding(f reasoning.Finding) {
	fmt.Fprintf(os.Stdout, "  [%s] %s\n", f.Severity, f.Detail)
	for _, e := range f.Evidence {
		fmt.Fprintf(os.Stdout, "      证据: %s\n", formatTriple(e))
	}
}

// filterBySeverity 按严重度筛选并按 subject 排序。
func filterBySeverity(findings []reasoning.Finding, sev string) []reasoning.Finding {
	var out []reasoning.Finding
	for _, f := range findings {
		if f.Severity == sev {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Subject.Value < out[j].Subject.Value })
	return out
}
