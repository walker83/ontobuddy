package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/walker/myonto/internal/rdf"
	"github.com/walker/myonto/internal/reasoning"
	"github.com/walker/myonto/internal/store"
)

// ReasonCmd 基于 RDFS/OWL 规则跑推理，推导出新的三元组。
//
// 默认 dry-run：只展示推导出的新三元组，不修改本体。
// 加 --apply 把新三元组物化进 ontology.ttl。
// 支持 --json：输出结构化结果供 LLM 消费。
type ReasonCmd struct {
	Apply bool `short:"a" help:"把推导出的新三元组物化进本体（默认仅展示）"`
	Limit int  `short:"n" help:"最多展示的推导三元组条数（0 = 不限）" placeholder:"N"`
	Reset bool `help:"清除所有之前 reason --apply 物化的推论（按 inferredBy 标记）"`
}

// reasonResult 是 --json 模式的输出结构。
type reasonResult struct {
	Saturated bool         `json:"saturated"`
	Derived   []jsonTriple `json:"derived"`
	WillApply bool         `json:"will_apply"`
	Applied   int          `json:"applied,omitempty"`
}

// jsonTriple 是三元组的 JSON 表示。
type jsonTriple struct {
	Subject   string         `json:"subject"`
	Predicate string         `json:"predicate"`
	Object    map[string]any `json:"object"`
}

// Run 执行推理。
func (c *ReasonCmd) Run() error {
	s, cfgPath, err := openStore()
	if err != nil {
		return err
	}
	// 处理 --reset：删所有带 inferredBy 标记的三元组
	if c.Reset {
		removed := 0
		// 找所有有 inferredBy 标记的 subject
		marked := map[rdf.Term]bool{}
		for _, t := range s.Query(rdf.Triple{Predicate: s.LocalIRI("inferredBy")}) {
			marked[t.Subject] = true
		}
		// 删除这些 subject 的所有三元组（它们都是推论产物）
		for subj := range marked {
			removed += s.Remove(rdf.Triple{Subject: subj})
		}
		// 还要删 inferredBy 标注自身
		s.Remove(rdf.Triple{Predicate: s.LocalIRI("inferredBy")})
		if removed == 0 {
			fmt.Fprintln(os.Stdout, "（无标记为推论的三元组）")
			return nil
		}
		if err := saveStore(s, cfgPath); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "✓ 已清除 %d 条推论三元组\n", removed)
		return nil
	}
	all := s.Triples()
	if len(all) == 0 {
		return reasonRender(c, cfgPath, s, nil)
	}

	r := reasoning.NewReasoner(all)
	derived := r.Derive()
	return reasonRender(c, cfgPath, s, derived)
}

// reasonRender 统一处理 --json / 人类可读两种输出。
func reasonRender(c *ReasonCmd, cfgPath string, s *store.Store, derived []rdf.Triple) error {
	if IsJSON() {
		out := reasonResult{
			Saturated: derived == nil || len(derived) == 0,
			Derived:   make([]jsonTriple, 0, len(derived)),
			WillApply: c.Apply,
		}
		for _, t := range derived {
			out.Derived = append(out.Derived, jsonTriple{
				Subject:   t.Subject.Value,
				Predicate: t.Predicate.Value,
				Object:    termToJSON(t.Object),
			})
		}

		if c.Apply && !out.Saturated {
			for _, t := range derived {
				s.Add(t)
				// 给推论加来源标记，便于 reason --reset 清除
				s.Add(rdf.Triple{Subject: t.Subject, Predicate: s.LocalIRI("inferredBy"), Object: rdf.Lit("reasoner:" + t.Predicate.LocalName())})
			}
			if err := saveStore(s, cfgPath); err != nil {
				return err
			}
			out.Applied = len(derived)
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	// 人类可读路径
	if derived == nil || len(derived) == 0 {
		if len(s.Triples()) == 0 {
			fmt.Fprintln(os.Stdout, "（本体为空，无可推理内容）")
		} else {
			fmt.Fprintln(os.Stdout, "（无新推导，原本体已饱和）")
		}
		return nil
	}

	fmt.Fprintf(os.Stdout, "推理共推导出 %d 条新三元组：\n", len(derived))

	// 展示用切片：可能被 --limit 截断；物化永远用全量 derived。
	// 两者必须分开，否则 `reason -a -n 5` 会只物化前 5 条而误以为写入了全部。
	shown := derived
	limit := c.Limit
	if limit > 0 && limit < len(derived) {
		fmt.Fprintf(os.Stdout, "（仅展示前 %d 条，加 -n 0 展示全部）\n", limit)
		shown = derived[:limit]
	}
	for _, t := range shown {
		fmt.Fprintf(os.Stdout, "  + %s\n", formatTriple(t))
	}

	if c.Apply {
		for _, t := range derived {
			s.Add(t)
			s.Add(rdf.Triple{Subject: t.Subject, Predicate: s.LocalIRI("inferredBy"), Object: rdf.Lit("reasoner")})
		}
		if err := saveStore(s, cfgPath); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "\n✓ 已物化 %d 条三元组到 %s（每条带 inferredBy 标记，可用 reason --reset 清除）\n", len(derived), s.Config().DataFile)
	} else {
		fmt.Fprintln(os.Stdout, "\n加 -a / --apply 把以上三元组写入本体")
	}
	return nil
}

// formatTriple 把三元组格式化为简洁可读形式。
func formatTriple(t rdf.Triple) string {
	return fmt.Sprintf("%s %s %s",
		shortTerm(t.Subject),
		shortTerm(t.Predicate),
		shortTerm(t.Object),
	)
}

// shortTerm 渲染 Term：IRI 用 local name，字面量带引号。
func shortTerm(t rdf.Term) string {
	switch t.Kind {
	case rdf.KindIRI:
		return t.LocalName()
	case rdf.KindLiteral:
		return `"` + t.Value + `"`
	case rdf.KindBlank:
		return "_:" + t.Value
	}
	return t.Value
}
