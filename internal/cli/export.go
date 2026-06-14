package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/walker/myonto/internal/rdf"
)

// ExportCmd 以多种格式导出当前本体。
//
// 主要用途：把本体塞给外部 LLM（Claude/opencode）当上下文。
// `--for-llm` 输出紧凑文本（同 schema + individuals 摘要），最适合塞 prompt。
type ExportCmd struct {
	ForLLM bool `short:"l" help:"输出紧凑文本格式，适合塞给 LLM 当上下文（默认人类可读 Turtle）"`
	AsJSON bool `short:"j" help:"输出 JSON-LD 风格的结构化 JSON"`
}

// Run 执行导出。
func (c *ExportCmd) Run() error {
	s, _, err := openStore()
	if err != nil {
		return err
	}
	// 全局 --json 优先级低于显式 -j/-l；这里尊重显式 flag。
	useJSON := c.AsJSON || (!c.ForLLM && IsJSON())

	if c.ForLLM {
		return exportForLLM(s)
	}
	if useJSON {
		return exportJSON(s)
	}
	// 默认：Turtle（完整三元组）
	return exportTurtle(s)
}

// exportForLLM 输出紧凑文本——给 LLM 当上下文用。
// 包含：schema 摘要 + 全部个体（local name + label + 类型 + 关键关系）。
func exportForLLM(s storeLike) error {
	schemaM := buildSchemaModel(s)

	var b strings.Builder
	fmt.Fprintf(&b, "# 本体上下文（%d 类 / %d 谓词 / %d 个体）\n\n",
		len(schemaM.Classes), len(schemaM.Properties), schemaM.Individuals)

	// 类
	b.WriteString("## 类\n")
	for _, c := range schemaM.Classes {
		line := "- " + c.ID
		if len(c.Parents) > 0 {
			line += " (parent: " + strings.Join(c.Parents, ", ") + ")"
		}
		if c.Count > 0 {
			line += fmt.Sprintf(" [%d 实例]", c.Count)
		}
		b.WriteString(line + "\n")
	}

	// 谓词
	b.WriteString("\n## 谓词\n")
	for _, p := range schemaM.Properties {
		line := "- " + p.ID + " (" + p.Type + ")"
		if p.Domain != "" || p.Range != "" {
			line += fmt.Sprintf(" [%s→%s]", or(p.Domain, "?"), or(p.Range, "?"))
		}
		if p.InverseOf != "" {
			line += " inv=" + p.InverseOf
		}
		b.WriteString(line + "\n")
	}

	// 个体摘要（每个一行）
	b.WriteString("\n## 个体\n")
	inds := collectIndividuals(s)
	for _, ind := range inds {
		parts := []string{ind.LocalName()}
		// label
		for _, t := range s.Query(rdf.Triple{Subject: ind, Predicate: rdf.Label}) {
			parts[0] = t.Object.Value + " (" + ind.LocalName() + ")"
			break
		}
		// 类型
		var types []string
		for _, t := range s.Query(rdf.Triple{Subject: ind, Predicate: rdf.Type}) {
			types = append(types, t.Object.LocalName())
		}
		if len(types) > 0 {
			parts = append(parts, "type="+strings.Join(types, ","))
		}
		// 关键关系（非 label/comment/type 的）
		var rels []string
		for _, t := range s.Query(rdf.Triple{Subject: ind}) {
			if t.Predicate.Equal(rdf.Label) || t.Predicate.Equal(rdf.Comment) || t.Predicate.Equal(rdf.Type) {
				continue
			}
			rel := t.Predicate.LocalName() + "="
			if t.Object.Kind == rdf.KindIRI {
				rel += t.Object.LocalName()
			} else {
				rel += `"` + truncate(t.Object.Value, 30) + `"`
			}
			rels = append(rels, rel)
		}
		if len(rels) > 0 {
			parts = append(parts, strings.Join(rels, " "))
		}
		// comment 简短
		for _, t := range s.Query(rdf.Triple{Subject: ind, Predicate: rdf.Comment}) {
			parts = append(parts, "# "+truncate(t.Object.Value, 80))
			break
		}
		fmt.Fprintln(&b, "- "+strings.Join(parts, " | "))
	}

	fmt.Fprint(os.Stdout, b.String())
	return nil
}

// exportJSON 输出 JSON 风格（个体 + 关系展开）。
func exportJSON(s storeLike) error {
	inds := collectIndividuals(s)
	type jsonEntity struct {
		Local     string           `json:"local"`
		IRI       string           `json:"iri"`
		Label     string           `json:"label,omitempty"`
		Types     []string         `json:"types,omitempty"`
		Desc      string           `json:"desc,omitempty"`
		Relations []map[string]any `json:"relations,omitempty"`
	}
	out := []jsonEntity{}
	for _, ind := range inds {
		e := jsonEntity{Local: ind.LocalName(), IRI: ind.Value}
		for _, t := range s.Query(rdf.Triple{Subject: ind, Predicate: rdf.Label}) {
			e.Label = t.Object.Value
		}
		for _, t := range s.Query(rdf.Triple{Subject: ind, Predicate: rdf.Comment}) {
			e.Desc = t.Object.Value
		}
		for _, t := range s.Query(rdf.Triple{Subject: ind, Predicate: rdf.Type}) {
			e.Types = append(e.Types, t.Object.LocalName())
		}
		for _, t := range s.Query(rdf.Triple{Subject: ind}) {
			if t.Predicate.Equal(rdf.Label) || t.Predicate.Equal(rdf.Comment) || t.Predicate.Equal(rdf.Type) {
				continue
			}
			rel := map[string]any{"pred": t.Predicate.LocalName()}
			if t.Object.Kind == rdf.KindIRI {
				rel["object"] = t.Object.LocalName()
				rel["object_iri"] = t.Object.Value
			} else {
				rel["object"] = t.Object.Value
				rel["type"] = "literal"
			}
			e.Relations = append(e.Relations, rel)
		}
		out = append(out, e)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"entities": out, "count": len(out)})
}

// exportTurtle 输出原始 Turtle（完整三元组）。
func exportTurtle(s storeLike) error {
	// 直接调 store 的序列化（但 storeLike 接口没暴露这个，得用 Triples + SerializeTurtle）
	triples := s.Triples()
	// 用 rdf 包的 SerializeTurtle
	fmt.Fprint(os.Stdout, rdf.SerializeTurtle(triples, nil))
	return nil
}

// collectIndividuals 收集所有"实例"（type 非 meta 的 subject）。
func collectIndividuals(s storeLike) []rdf.Term {
	metaObjects := map[rdf.Term]bool{
		rdf.Class: true, rdf.OwlClass: true,
		rdf.OwlObjectProperty: true,
		rdf.IRI("http://www.w3.org/2002/07/owl#DatatypeProperty"): true,
		rdf.TransitiveProperty: true, rdf.SymmetricProperty: true,
	}
	set := map[rdf.Term]bool{}
	for _, t := range s.Triples() {
		if t.Predicate.Equal(rdf.Type) && !metaObjects[t.Object] {
			set[t.Subject] = true
		}
	}
	out := make([]rdf.Term, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LocalName() < out[j].LocalName() })
	return out
}
