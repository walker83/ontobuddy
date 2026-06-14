package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/walker/myonto/internal/rdf"
)

// SchemaCmd 输出当前本体的元模型（schema 自省）。
//
// 这是给外部 LLM / Agent 用的——它们在生成新实体前，应先调 schema
// 看现有有哪些类、谓词、约束，从而复用而非冲突。
//
// 默认人类可读；--json 输出结构化 JSON。
type SchemaCmd struct{}

// Run 输出 schema。
func (c *SchemaCmd) Run() error {
	s, _, err := openStore()
	if err != nil {
		return err
	}
	m := buildSchemaModel(s)

	if IsJSON() {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(m)
	}
	printSchemaHuman(m)
	return nil
}

// schemaModel 是 schema 的结构化表示。
type schemaModel struct {
	Classes      []schemaClass    `json:"classes"`
	Properties   []schemaProperty `json:"properties"`
	Individuals  int              `json:"individuals"`
	TotalTriples int              `json:"total_triples"`
}

type schemaClass struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Comment  string   `json:"comment,omitempty"`
	Parents  []string `json:"parents,omitempty"`  // rdfs:subClassOf 的父类 local name
	Children []string `json:"children,omitempty"` // 直接子类（计算得出）
	Count    int      `json:"count"`              // 该类的直接实例数（不含子类继承的）
}

type schemaProperty struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Comment   string `json:"comment,omitempty"`
	Type      string `json:"type"`                 // "object" / "datatype" / "symmetric" / "transitive"
	Domain    string `json:"domain,omitempty"`     // local name
	Range     string `json:"range,omitempty"`      // local name 或 xsd 类型
	InverseOf string `json:"inverse_of,omitempty"` // 逆属性的 local name
}

// buildSchemaModel 从 store 构造 schema 视图。
func buildSchemaModel(s storeLike) schemaModel {
	all := s.Triples()
	model := schemaModel{TotalTriples: len(all)}

	// 扫描所有类定义
	classIDs := []rdf.Term{}
	classSet := map[rdf.Term]bool{}
	for _, t := range all {
		if t.Predicate.Equal(rdf.Type) && (t.Object.Equal(rdf.Class) || t.Object.Equal(rdf.OwlClass)) {
			if !classSet[t.Subject] {
				classSet[t.Subject] = true
				classIDs = append(classIDs, t.Subject)
			}
		}
	}

	// 给每个类收集 label/comment/parents/instanceCount
	for _, cid := range classIDs {
		sc := schemaClass{ID: cid.LocalName()}
		for _, t := range s.Query(rdf.Triple{Subject: cid, Predicate: rdf.Label}) {
			sc.Label = t.Object.Value
		}
		if sc.Label == "" {
			sc.Label = cid.LocalName()
		}
		for _, t := range s.Query(rdf.Triple{Subject: cid, Predicate: rdf.Comment}) {
			sc.Comment = t.Object.Value
		}
		for _, t := range s.Query(rdf.Triple{Subject: cid, Predicate: rdf.SubClassOf}) {
			sc.Parents = append(sc.Parents, t.Object.LocalName())
		}
		// 直接实例计数：type == cid 且 subject 不是 cid 自身（避免把类定义算进去）
		for _, t := range s.Query(rdf.Triple{Predicate: rdf.Type, Object: cid}) {
			if !t.Subject.Equal(cid) {
				sc.Count++
			}
		}
		model.Classes = append(model.Classes, sc)
	}

	// 计算 children（反向看 parents）
	parentToChildren := map[string][]string{}
	for _, c := range model.Classes {
		for _, p := range c.Parents {
			parentToChildren[p] = append(parentToChildren[p], c.ID)
		}
	}
	for i := range model.Classes {
		if kids, ok := parentToChildren[model.Classes[i].ID]; ok {
			model.Classes[i].Children = kids
		}
	}
	sort.Slice(model.Classes, func(i, j int) bool { return model.Classes[i].ID < model.Classes[j].ID })

	// 扫描所有属性定义（owl:ObjectProperty / owl:DatatypeProperty / 对称 / 逆）
	propSet := map[rdf.Term]bool{}
	for _, t := range all {
		if t.Predicate.Equal(rdf.Type) {
			if t.Object.Equal(rdf.OwlObjectProperty) || t.Object.Equal(rdf.IRI("http://www.w3.org/2002/07/owl#DatatypeProperty")) ||
				t.Object.Equal(rdf.TransitiveProperty) || t.Object.Equal(rdf.SymmetricProperty) ||
				t.Object.Equal(rdf.IRI("http://www.w3.org/2002/07/owl#FunctionalProperty")) {
				if !propSet[t.Subject] && !classSet[t.Subject] {
					propSet[t.Subject] = true
				}
			}
		}
	}

	for pid := range propSet {
		sp := schemaProperty{ID: pid.LocalName(), Label: pid.LocalName(), Type: "object"}
		for _, t := range s.Query(rdf.Triple{Subject: pid, Predicate: rdf.Label}) {
			sp.Label = t.Object.Value
		}
		for _, t := range s.Query(rdf.Triple{Subject: pid, Predicate: rdf.Comment}) {
			sp.Comment = t.Object.Value
		}
		// domain
		for _, t := range s.Query(rdf.Triple{Subject: pid, Predicate: rdf.IRI("http://www.w3.org/2000/01/rdf-schema#domain")}) {
			sp.Domain = t.Object.LocalName()
		}
		// range
		for _, t := range s.Query(rdf.Triple{Subject: pid, Predicate: rdf.IRI("http://www.w3.org/2000/01/rdf-schema#range")}) {
			sp.Range = t.Object.LocalName()
		}
		// 类型
		for _, t := range s.Query(rdf.Triple{Subject: pid, Predicate: rdf.Type}) {
			if t.Object.Equal(rdf.IRI("http://www.w3.org/2002/07/owl#DatatypeProperty")) {
				sp.Type = "datatype"
			}
			if t.Object.Equal(rdf.SymmetricProperty) {
				sp.Type = "symmetric"
			}
			if t.Object.Equal(rdf.TransitiveProperty) {
				sp.Type = "transitive"
			}
		}
		// inverseOf
		for _, t := range s.Query(rdf.Triple{Subject: pid, Predicate: rdf.InverseOf}) {
			sp.InverseOf = t.Object.LocalName()
		}
		model.Properties = append(model.Properties, sp)
	}
	// 也扫反向 inverseOf（B inverseOf A 时 A 也应有标记）
	for _, t := range all {
		if t.Predicate.Equal(rdf.InverseOf) {
			// 检查 t.Object 是否已在 model.Properties
			found := false
			for i := range model.Properties {
				if model.Properties[i].ID == t.Object.LocalName() {
					if model.Properties[i].InverseOf == "" {
						model.Properties[i].InverseOf = t.Subject.LocalName()
					}
					found = true
					break
				}
			}
			if !found {
				model.Properties = append(model.Properties, schemaProperty{
					ID:        t.Object.LocalName(),
					Label:     t.Object.LocalName(),
					Type:      "object",
					InverseOf: t.Subject.LocalName(),
				})
			}
		}
	}
	sort.Slice(model.Properties, func(i, j int) bool { return model.Properties[i].ID < model.Properties[j].ID })

	// individuals 计数：所有 type 不是 rdfs:Class/owl:Class/ObjectProperty/... 的 subject
	individualSet := map[rdf.Term]bool{}
	metaObjects := map[rdf.Term]bool{
		rdf.Class: true, rdf.OwlClass: true,
		rdf.OwlObjectProperty: true,
		rdf.IRI("http://www.w3.org/2002/07/owl#DatatypeProperty"): true,
		rdf.TransitiveProperty: true, rdf.SymmetricProperty: true,
	}
	for _, t := range all {
		if t.Predicate.Equal(rdf.Type) && !metaObjects[t.Object] {
			individualSet[t.Subject] = true
		}
	}
	model.Individuals = len(individualSet)
	return model
}

// printSchemaHuman 人类可读输出。
func printSchemaHuman(m schemaModel) {
	fmt.Fprintf(os.Stdout, "本体 Schema（%d 个类 / %d 个谓词 / %d 个个体 / %d 条三元组）\n\n",
		len(m.Classes), len(m.Properties), m.Individuals, m.TotalTriples)

	fmt.Fprintln(os.Stdout, "=== 类层级 ===")
	// 先找根类（无父类）
	roots := []schemaClass{}
	for _, c := range m.Classes {
		if len(c.Parents) == 0 {
			roots = append(roots, c)
		}
	}
	for _, r := range roots {
		printClassTree(m, r, 0)
	}

	fmt.Fprintln(os.Stdout, "\n=== 谓词 ===")
	for _, p := range m.Properties {
		line := fmt.Sprintf("  %-20s  %s", p.ID, p.Type)
		if p.Domain != "" || p.Range != "" {
			line += fmt.Sprintf("  [%s → %s]", or(p.Domain, "?"), or(p.Range, "?"))
		}
		if p.InverseOf != "" {
			line += fmt.Sprintf("  (inverse: %s)", p.InverseOf)
		}
		fmt.Fprintln(os.Stdout, line)
		if p.Comment != "" {
			fmt.Fprintf(os.Stdout, "    %s\n", truncate(p.Comment, 80))
		}
	}
}

func printClassTree(m schemaModel, c schemaClass, depth int) {
	indent := strings.Repeat("  ", depth)
	count := ""
	if c.Count > 0 {
		count = fmt.Sprintf(" (%d 实例)", c.Count)
	}
	fmt.Fprintf(os.Stdout, "%s- %s%s%s\n", indent, c.ID, count, orComment(c.Comment))
	// 找子类
	for _, k := range m.Classes {
		for _, p := range k.Parents {
			if p == c.ID {
				printClassTree(m, k, depth+1)
			}
		}
	}
}

func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func orComment(s string) string {
	if s == "" {
		return ""
	}
	return "  # " + truncate(s, 60)
}

// storeLike 是 store.Store 的最小接口（避免循环导入 + 便于测试）。
type storeLike interface {
	Triples() []rdf.Triple
	Query(rdf.Triple) []rdf.Triple
}
