package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

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
	// 处理 --reset：移除 reason --apply 物化的推论三元组。
	// 策略：遍历所有 inferredBy 标记，解码出原推论三元组并精确删除它 + 标记本身。
	// 这避免了两个历史陷阱：
	//   (a) 按 subject 粗删——会误删该 subject 的原始数据（如 newton a Scientist）；
	//   (b) "重跑 Derive"——在已含物化数据的 store 上 Derive 必然返回空（推论已被当原始）。
	// 可逆编码（encodeInferredBy）让我们能从标记精确还原三元组。
	if c.Reset {
		removed, err := resetInferred(s)
		if err != nil {
			return err
		}
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
			n, err := materialize(s, cfgPath, derived)
			if err != nil {
				return err
			}
			out.Applied = n
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
		n, err := materialize(s, cfgPath, derived)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "\n✓ 已物化 %d 条三元组到 %s（每条带 inferredBy 标记，可用 reason --reset 清除）\n", n, s.Config().DataFile)
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

// materialize 把推导出的三元组写入 store，并给每条附 inferredBy 来源标记，
// 然后落盘。返回实际物化的条数。
//
// inferredBy 标记设计成**可逆**的：<t.Subject> <inferredBy> "<encoded>"，
// 其中 encoded 编码推论的 predicate+object（subject 已在标记主语位置）。
// 这样 reason --reset 能精确重建并删除被物化的推论，而不误删原始数据，
// 也不依赖"重跑 Derive"（后者在已物化数据上注定返回空，见历史 bug）。
//
// 编码格式：reasoner\t<predicate-iri>\t<object-canonical>
// 用 Tab 分隔避免与 IRI/字面量内容冲突；object 用 Term.String() 规范形式。
// 历史上两条输出路径 inferredBy 取值不一致（"reasoner:<pred>" vs 裸 "reasoner"），
// 抽成单函数从根上消除分叉。
func materialize(s *store.Store, cfgPath string, derived []rdf.Triple) (int, error) {
	inferredByPred := s.LocalIRI("inferredBy")
	for _, t := range derived {
		s.Add(t)
		s.Add(rdf.Triple{
			Subject:   t.Subject,
			Predicate: inferredByPred,
			Object:    rdf.Lit(encodeInferredBy(t)),
		})
	}
	if err := saveStore(s, cfgPath); err != nil {
		return 0, err
	}
	return len(derived), nil
}

// encodeInferredBy 把一条推论三元组编码成 inferredBy 标记的字面量值。
// 格式：reasoner\t<predicate-IRI>\t<object-canonical>
func encodeInferredBy(t rdf.Triple) string {
	return "reasoner\t" + t.Predicate.Value + "\t" + t.Object.String()
}

// decodeInferredBy 反编码 inferredBy 标记，结合标记的 subject 重建原推论三元组。
// 输入：markSubject（标记的主语，即推论的 subject）+ markValue（标记的字面量值）。
// 无法解析（旧格式或损坏）时返回 ok=false。
func decodeInferredBy(markSubject rdf.Term, markValue string) (rdf.Triple, bool) {
	// 兼容旧格式："reasoner" 或 "reasoner:<predLocal>"——无法还原完整三元组，
	// 返回 ok=false 让 reset 回退到安全策略（见 resetInferred）。
	if !strings.HasPrefix(markValue, "reasoner\t") {
		return rdf.Triple{}, false
	}
	rest := markValue[len("reasoner\t"):]
	tabIdx := strings.Index(rest, "\t")
	if tabIdx < 0 {
		return rdf.Triple{}, false
	}
	predIRI := rest[:tabIdx]
	objCanonical := rest[tabIdx+1:]
	obj, ok := parseCanonicalTerm(objCanonical)
	if !ok {
		return rdf.Triple{}, false
	}
	return rdf.Triple{
		Subject:   markSubject,
		Predicate: rdf.IRI(predIRI),
		Object:    obj,
	}, true
}

// parseCanonicalTerm 把 Term.String() 的输出反解析回 Term。
// 支持 IRI <...>、blank _:...、字面量 "..."[@lang|^^<datatype>]。
func parseCanonicalTerm(s string) (rdf.Term, bool) {
	if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
		return rdf.IRI(s[1 : len(s)-1]), true
	}
	if strings.HasPrefix(s, "_:") {
		return rdf.Blank(s[2:]), true
	}
	if strings.HasPrefix(s, `"`) {
		// 找到结束引号（处理转义）。
		i := 1
		for i < len(s) {
			if s[i] == '\\' && i+1 < len(s) {
				i += 2
				continue
			}
			if s[i] == '"' {
				break
			}
			i++
		}
		if i >= len(s) {
			return rdf.Term{}, false
		}
		value := unescapeLiteral(s[1:i])
		rest := s[i+1:]
		if strings.HasPrefix(rest, "@") {
			return rdf.LangLit(value, rest[1:]), true
		}
		if strings.HasPrefix(rest, "^^<") && strings.HasSuffix(rest, ">") {
			return rdf.TypedLit(value, rest[3:len(rest)-1]), true
		}
		return rdf.Lit(value), true
	}
	return rdf.Term{}, false
}

// unescapeLiteral 反转 escapeLiteral 的转义。与 rdf.escapeLiteral 对应。
func unescapeLiteral(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case '\\':
				b.WriteByte('\\')
			case '"':
				b.WriteByte('"')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte('\\')
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// resetInferred 清除所有 reason --apply 物化的推论三元组及其 inferredBy 标记。
//
// 对每条 inferredBy 标记：
//   - 新格式（reasoner\t<iri>\t<obj>）：解码出原推论三元组，精确删除它 + 标记本身。
//   - 旧格式（裸 "reasoner" 或 "reasoner:<predLocal>"）：无法安全还原完整三元组，
//     仅删除标记本身（推论三元组保留——宁可漏删也不误删原始数据），并计为 legacy。
//
// 返回：成功删除的推论条数（不含标记本身）。
func resetInferred(s *store.Store) (int, error) {
	inferredByPred := s.LocalIRI("inferredBy")
	marks := s.Query(rdf.Triple{Predicate: inferredByPred})
	if len(marks) == 0 {
		return 0, nil
	}
	removed := 0
	for _, mark := range marks {
		// 无论能否解码，标记本身都要删。
		s.Remove(mark)
		// 尝试解码并删除原推论三元组。
		if triple, ok := decodeInferredBy(mark.Subject, mark.Object.Value); ok {
			removed += s.Remove(triple)
		}
		// 旧格式：仅删标记，无法定位推论三元组（见函数 doc）。
	}
	return removed, nil
}
