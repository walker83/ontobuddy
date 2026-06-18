package eval

import (
	"fmt"
	"io"
	"strings"
)

// Report 聚合多个 case 的结果。
type Report struct {
	Results []Result
	// 汇总指标（所有 case 的 TP/FP/FN 加总后算 P/R/F1，即 micro-average）。
	TotalTP, TotalFP, TotalFN int
	Precision                 float64
	Recall                    float64
	F1                        float64
	Passed                    int
	Failed                    int
}

// Aggregate 把多个 Result 聚合成 Report。
// 用 micro-average：把所有 case 的 TP/FP/FN 加总后再算 P/R/F1，
// 这样大 case 不会被小 case 平均稀释。
func Aggregate(results []Result) Report {
	rep := Report{Results: results}
	for _, r := range results {
		rep.TotalTP += r.TP
		rep.TotalFP += r.FP
		rep.TotalFN += r.FN
		if r.Pass() {
			rep.Passed++
		} else {
			rep.Failed++
		}
	}
	rep.Precision, rep.Recall, rep.F1 = computeMetrics(rep.TotalTP, rep.TotalFP, rep.TotalFN)
	return rep
}

// Pass 当且仅当所有 case 都过（Failed==0）且全局 P==R==1.0。
// 这是 CI 的硬门槛。
func (rep Report) Pass() bool {
	return rep.Failed == 0 && rep.TotalFP == 0 && rep.TotalFN == 0
}

// WriteHuman 把人类可读报告写到 w。
func (rep Report) WriteHuman(w io.Writer) {
	fmt.Fprintln(w, "════════════════════════════════════════════════════════════")
	fmt.Fprintf(w, "推理评估报告：%d 个 case（通过 %d / 失败 %d）\n",
		len(rep.Results), rep.Passed, rep.Failed)
	fmt.Fprintf(w, "Micro-avg: Precision=%.4f  Recall=%.4f  F1=%.4f  (TP=%d FP=%d FN=%d)\n",
		rep.Precision, rep.Recall, rep.F1, rep.TotalTP, rep.TotalFP, rep.TotalFN)
	fmt.Fprintln(w, "────────────────────────────────────────────────────────────")
	for _, r := range rep.Results {
		fmt.Fprintln(w, r.String())
	}
	status := "✓ ALL PASS"
	if !rep.Pass() {
		status = "✗ HAS FAILURES"
	}
	fmt.Fprintln(w, "────────────────────────────────────────────────────────────")
	fmt.Fprintf(w, "%s\n", status)
}

// String 便于测试断言与日志。
func (rep Report) String() string {
	var b strings.Builder
	rep.WriteHuman(&b)
	return b.String()
}
