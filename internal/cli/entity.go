package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/walker/myonto/internal/config"
	"github.com/walker/myonto/internal/model"
	"github.com/walker/myonto/internal/rdf"
	"github.com/walker/myonto/internal/store"
)

// EntityCmd 是 myonto entity 的命令组。
type EntityCmd struct {
	Add      EntityAddCmd      `cmd:"" help:"添加一个个体实体（如 isaac-newton）"`
	AddClass EntityAddClassCmd `cmd:"" help:"添加一个类（rdfs:Class，如 Person）"`
	List     EntityListCmd     `cmd:"" help:"列出本体中所有实体，可按类型/标签过滤"`
	Show     EntityShowCmd     `cmd:"" help:"查看某实体的全部三元组（属性与关系）"`
	Edit     EntityEditCmd     `cmd:"" help:"修改实体的描述或类型"`
	Rm       EntityRmCmd       `cmd:"" help:"删除实体（及其作为主语的所有三元组）"`
	Apply    EntityApplyCmd    `cmd:"" help:"批量应用 JSON 数组（外部 LLM 产出，含实体定义和关系三元组）"`
}

// --- entity add ---

type EntityAddCmd struct {
	Name string   `arg:"" required:"" placeholder:"NAME" help:"实体名（会被 slug 化作为 local name）"`
	Type string   `short:"t" help:"实体的类型（类名或 IRI）" placeholder:"CLASS"`
	Desc string   `short:"d" help:"实体的描述" placeholder:"TEXT"`
	Tag  []string `short:"g" help:"标签，可重复多次" placeholder:"TAG"`
	Attr []string `help:"自定义属性 key=value（可多次）；value 自动识别 dateTime/decimal/boolean 类型" placeholder:"KEY=VALUE"`
}

func (c *EntityAddCmd) Run() error {
	s, cfgPath, err := openStore()
	if err != nil {
		return err
	}
	local := model.Slug(c.Name)
	subj := s.LocalIRI(local)
	if len(s.Query(rdf.Triple{Subject: subj, Predicate: rdf.Label})) > 0 ||
		len(s.Query(rdf.Triple{Subject: subj, Predicate: rdf.Type})) > 0 {
		return fmt.Errorf("实体 %s 已存在", local)
	}
	s.Add(rdf.Triple{Subject: subj, Predicate: rdf.Label, Object: rdf.Lit(c.Name)})
	if c.Desc != "" {
		s.Add(rdf.Triple{Subject: subj, Predicate: rdf.Comment, Object: rdf.Lit(c.Desc)})
	}
	if c.Type != "" {
		t, err := s.ResolveName(c.Type)
		if err != nil {
			return fmt.Errorf("解析类型: %w", err)
		}
		s.Add(rdf.Triple{Subject: subj, Predicate: rdf.Type, Object: t})
	}
	for _, tag := range c.Tag {
		s.Add(rdf.Triple{Subject: subj, Predicate: s.LocalIRI("tag"), Object: rdf.Lit(tag)})
	}
	// 自定义属性：--attr key=value
	for _, a := range c.Attr {
		k, v, ok := splitKV(a)
		if !ok {
			return fmt.Errorf("无效 --attr %q（应为 key=value 格式）", a)
		}
		pred, err := s.ResolveName(k)
		if err != nil {
			pred = s.LocalIRI(model.Slug(k))
		}
		s.Add(rdf.Triple{Subject: subj, Predicate: pred, Object: autotypeLiteral(v)})
	}
	if err := saveStore(s, cfgPath); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "已添加个体：%s <%s>\n", c.Name, subj.Value)
	return nil
}

// --- entity add-class ---

type EntityAddClassCmd struct {
	Name   string `arg:"" required:"" placeholder:"NAME" help:"类名（会被 slug 化）"`
	Parent string `short:"p" help:"父类" placeholder:"CLASS"`
	Desc   string `short:"d" help:"类的描述" placeholder:"TEXT"`
}

func (c *EntityAddClassCmd) Run() error {
	s, cfgPath, err := openStore()
	if err != nil {
		return err
	}
	local := model.Slug(c.Name)
	subj := s.LocalIRI(local)
	if s.Has(rdf.Triple{Subject: subj, Predicate: rdf.Type, Object: rdf.Class}) {
		return fmt.Errorf("类 %s 已存在", local)
	}
	s.Add(rdf.Triple{Subject: subj, Predicate: rdf.Type, Object: rdf.Class})
	s.Add(rdf.Triple{Subject: subj, Predicate: rdf.Label, Object: rdf.Lit(c.Name)})
	if c.Desc != "" {
		s.Add(rdf.Triple{Subject: subj, Predicate: rdf.Comment, Object: rdf.Lit(c.Desc)})
	}
	if c.Parent != "" {
		p, err := s.ResolveName(c.Parent)
		if err != nil {
			return fmt.Errorf("解析父类: %w", err)
		}
		s.Add(rdf.Triple{Subject: subj, Predicate: rdf.SubClassOf, Object: p})
	}
	if err := saveStore(s, cfgPath); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "已添加类：%s\n", subj.Value)
	return nil
}

// --- entity list ---

type EntityListCmd struct {
	Type string `short:"t" help:"只列出该类型的实例" placeholder:"CLASS"`
	Tag  string `short:"g" help:"只列出带此标签的实体" placeholder:"TAG"`
}

func (c *EntityListCmd) Run() error {
	s, _, err := openStore()
	if err != nil {
		return err
	}
	subjects := s.Subjects()
	if len(subjects) == 0 {
		if IsJSON() {
			printEntitySummariesJSON(nil)
		} else {
			fmt.Fprintln(os.Stdout, "（本体为空）")
		}
		return nil
	}
	typeFilter := rdf.Term{}
	if c.Type != "" {
		typeFilter, err = s.ResolveName(c.Type)
		if err != nil {
			return err
		}
	}
	var summaries []EntitySummary
	rows := 0
	for _, subj := range subjects {
		if c.Type != "" {
			ts := s.Query(rdf.Triple{Subject: subj, Predicate: rdf.Type, Object: typeFilter})
			if len(ts) == 0 {
				continue
			}
		}
		if c.Tag != "" {
			ts := s.Query(rdf.Triple{Subject: subj, Predicate: s.LocalIRI("tag"), Object: rdf.Lit(c.Tag)})
			if len(ts) == 0 {
				continue
			}
		}
		if IsJSON() {
			summaries = append(summaries, buildSummary(s, subj))
		} else {
			label := labelOf(s, subj)
			typeStr := typeStringOf(s, subj)
			fmt.Fprintf(os.Stdout, "%-24s  %s\n", subj.LocalName(), decorate(label, typeStr))
		}
		rows++
	}
	if IsJSON() {
		printEntitySummariesJSON(summaries)
		return nil
	}
	if rows == 0 {
		fmt.Fprintln(os.Stdout, "（无匹配实体）")
	}
	return nil
}

// buildSummary 从 store 构造 EntitySummary（list/search 复用）。
func buildSummary(s *store.Store, subj rdf.Term) EntitySummary {
	types := typeStringList(s, subj)
	label := labelOf(s, subj)
	desc := ""
	for _, t := range s.Query(rdf.Triple{Subject: subj, Predicate: rdf.Comment}) {
		desc = t.Object.Value
		break
	}
	return EntitySummary{
		Local: subj.LocalName(),
		IRI:   subj.Value,
		Label: label,
		Types: types,
		Desc:  desc,
	}
}

// typeStringList 取出所有类型 local name（rdfs:Class 标 (class)）。
func typeStringList(s *store.Store, subj rdf.Term) []string {
	var out []string
	for _, t := range s.Query(rdf.Triple{Subject: subj, Predicate: rdf.Type}) {
		if t.Object.Equal(rdf.Class) {
			out = append(out, "(class)")
			continue
		}
		out = append(out, t.Object.LocalName())
	}
	return out
}

// --- entity show ---

type EntityShowCmd struct {
	Name string `arg:"" required:"" placeholder:"NAME" help:"实体名"`
}

func (c *EntityShowCmd) Run() error {
	s, _, err := openStore()
	if err != nil {
		return err
	}
	subj, err := s.ResolveName(c.Name)
	if err != nil {
		return err
	}
	triples := s.Query(rdf.Triple{Subject: subj})
	if len(triples) == 0 {
		return fmt.Errorf("未找到实体 %s", c.Name)
	}
	if IsJSON() {
		out := make([]map[string]any, 0, len(triples))
		for _, t := range triples {
			out = append(out, map[string]any{
				"subject":   t.Subject.Value,
				"predicate": t.Predicate.Value,
				"object":    termToJSON(t.Object),
			})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"entity":  subj.Value,
			"triples": out,
			"count":   len(out),
		})
	}
	fmt.Fprintf(os.Stdout, "<%s>\n", subj.Value)
	for _, t := range triples {
		fmt.Fprintf(os.Stdout, "  %-16s %s\n", t.Predicate.LocalName(), formatObject(t.Object))
	}
	return nil
}

// termToJSON 把 rdf.Term 序列化成 JSON 友好结构。
func termToJSON(t rdf.Term) map[string]any {
	switch t.Kind {
	case rdf.KindIRI:
		return map[string]any{"type": "iri", "value": t.Value}
	case rdf.KindLiteral:
		m := map[string]any{"type": "literal", "value": t.Value}
		if t.Lang != "" {
			m["lang"] = t.Lang
		} else if t.DataType != "" && t.DataType != rdf.XSDString {
			m["datatype"] = t.DataType
		}
		return m
	case rdf.KindBlank:
		return map[string]any{"type": "blank", "value": t.Value}
	}
	return map[string]any{"value": t.Value}
}

// --- entity edit ---

type EntityEditCmd struct {
	Name string   `arg:"" required:"" placeholder:"NAME" help:"实体名"`
	Desc *string  `short:"d" help:"新的描述；传空串清除" placeholder:"TEXT"`
	Type []string `short:"t" help:"新的类型（覆盖原有）" placeholder:"CLASS"`
}

func (c *EntityEditCmd) Run() error {
	s, cfgPath, err := openStore()
	if err != nil {
		return err
	}
	subj, err := s.ResolveName(c.Name)
	if err != nil {
		return err
	}
	if len(s.Query(rdf.Triple{Subject: subj})) == 0 {
		return fmt.Errorf("未找到实体 %s", c.Name)
	}
	if c.Desc != nil {
		s.Remove(rdf.Triple{Subject: subj, Predicate: rdf.Comment})
		if *c.Desc != "" {
			s.Add(rdf.Triple{Subject: subj, Predicate: rdf.Comment, Object: rdf.Lit(*c.Desc)})
		}
	}
	if len(c.Type) > 0 {
		s.Remove(rdf.Triple{Subject: subj, Predicate: rdf.Type})
		for _, tn := range c.Type {
			tt, err := s.ResolveName(tn)
			if err != nil {
				return fmt.Errorf("解析类型 %q: %w", tn, err)
			}
			s.Add(rdf.Triple{Subject: subj, Predicate: rdf.Type, Object: tt})
		}
	}
	if err := saveStore(s, cfgPath); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "已更新：%s\n", subj.Value)
	return nil
}

// --- entity rm ---

type EntityRmCmd struct {
	Name  string `arg:"" required:"" placeholder:"NAME" help:"实体名"`
	Force bool   `short:"f" help:"跳过确认直接删除"`
}

func (c *EntityRmCmd) Run() error {
	s, cfgPath, err := openStore()
	if err != nil {
		return err
	}
	subj, err := s.ResolveName(c.Name)
	if err != nil {
		return err
	}
	// 先统计：Query 不修改 store，安全用于确认前的数量展示。
	// 用 Remove 直接统计会让删除在确认前发生（即便没 save 也会污染内存 store）。
	n := len(s.Query(rdf.Triple{Subject: subj}))
	if n == 0 {
		return fmt.Errorf("未找到实体 %s", c.Name)
	}
	if !c.Force {
		return fmt.Errorf("将删除 %d 条三元组，加 -f 确认执行", n)
	}
	// 确认后才真正删除。
	removed := s.Remove(rdf.Triple{Subject: subj})
	if err := saveStore(s, cfgPath); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "已删除 %s（%d 条三元组）\n", subj.Value, removed)
	return nil
}

// --- 共用辅助 ---

func openStore() (*store.Store, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	dir, cfg, err := config.Find(cwd)
	if err != nil {
		return nil, "", err
	}
	s := store.New(cfg)
	dataPath := filepath.Join(dir, cfg.DataFile)
	if err := s.LoadFile(dataPath); err != nil {
		return nil, "", err
	}
	return s, dir, nil
}

func saveStore(s *store.Store, dir string) error {
	cfg := s.Config()
	dataPath := filepath.Join(dir, cfg.DataFile)
	return s.SaveFile(dataPath)
}

func labelOf(s *store.Store, subj rdf.Term) string {
	ts := s.Query(rdf.Triple{Subject: subj, Predicate: rdf.Label})
	if len(ts) > 0 {
		return ts[0].Object.Value
	}
	return subj.LocalName()
}

func typeStringOf(s *store.Store, subj rdf.Term) string {
	return strings.Join(typeStringList(s, subj), ", ")
}

func decorate(label, typeStr string) string {
	if typeStr == "" {
		return label
	}
	return label + "  [" + typeStr + "]"
}

func formatObject(t rdf.Term) string {
	switch t.Kind {
	case rdf.KindLiteral:
		return t.Value
	case rdf.KindIRI:
		return t.LocalName()
	case rdf.KindBlank:
		return "_:" + t.Value
	}
	return t.Value
}
