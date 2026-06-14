package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/walker/myonto/internal/model"
	"github.com/walker/myonto/internal/rdf"
)

// EntityApplyCmd 批量应用实体（外部 LLM/脚本产出的 JSON 数组）。
//
// 用途：外部 Claude/opencode 读笔记 → 产出 JSON → myonto entity apply --file <json>。
// 这替代了"跑 N 次 entity add"，且 JSON 格式稳定适合程序产出。
//
// 输入 JSON 格式（数组）：
//
//	[
//	  {"name": "王韬", "type": "Person", "label": "王韬", "desc": "..."},
//	  {"subject": "wang-tao", "pred": "marriedTo", "object": "deng-huan"},
//	  {"subject": "wang-tao", "pred": "atTime", "object": "2026-05-20", "datatype": "dateTime"}
//	]
//
// 每个元素要么是实体定义（含 name/type/desc），要么是关系三元组（subject/pred/object）。
type EntityApplyCmd struct {
	File string `arg:"" optional:"" placeholder:"FILE" help:"JSON 文件路径；省略则从 stdin 读"`
	Dry  bool   `help:"只展示会加什么，不写盘"`
}

// entityApplyItem 是 JSON 数组的元素 schema。
type entityApplyItem struct {
	// 实体定义模式
	Name  string `json:"name,omitempty"`  // 实体名（slug 化为 local name）
	Type  string `json:"type,omitempty"`  // 类名
	Label string `json:"label,omitempty"` // rdfs:label（可空，默认用 name）
	Desc  string `json:"desc,omitempty"`  // rdfs:comment

	// 关系三元组模式
	Subject   string `json:"subject,omitempty"`  // 主语 local name 或 IRI
	Predicate string `json:"pred,omitempty"`     // 谓词
	Object    string `json:"object,omitempty"`   // 宾语
	Datatype  string `json:"datatype,omitempty"` // 宾语的数据类型（如 dateTime/decimal/boolean）；空=当 IRI
}

// Run 执行批量 apply。
func (c *EntityApplyCmd) Run() error {
	var data []byte
	var err error
	if c.File != "" && c.File != "-" {
		data, err = os.ReadFile(c.File)
	} else {
		data, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		return fmt.Errorf("读输入: %w", err)
	}

	var items []entityApplyItem
	if err := json.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("解析 JSON: %w（期望数组 [{...}, ...]）", err)
	}

	s, cfgPath, err := openStore()
	if err != nil {
		return err
	}

	added := 0
	skipped := 0
	for i, item := range items {
		triples, err := itemToTriples(s, item)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [%d] 跳过: %v\n", i, err)
			skipped++
			continue
		}
		for _, t := range triples {
			if s.Has(t) {
				skipped++
				continue
			}
			s.Add(t)
			added++
		}
	}

	if c.Dry {
		fmt.Fprintf(os.Stdout, "[dry-run] 将添加 %d 条三元组（跳过 %d 条已存在/无效）\n", added, skipped)
		return nil
	}

	if added == 0 {
		fmt.Fprintf(os.Stdout, "无新三元组（跳过 %d 条）\n", skipped)
		return nil
	}
	if err := saveStore(s, cfgPath); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "✓ 已添加 %d 条三元组（跳过 %d 条）\n", added, skipped)
	return nil
}

// itemToTriples 把一个 JSON 项转成三元组列表。
func itemToTriples(s resolveStore, item entityApplyItem) ([]rdf.Triple, error) {
	// 模式 1：实体定义（有 name）
	if item.Name != "" {
		subj := s.LocalIRI(model.Slug(item.Name))
		var out []rdf.Triple
		out = append(out, rdf.Triple{Subject: subj, Predicate: rdf.Label, Object: rdf.Lit(orDefault(item.Label, item.Name))})
		if item.Desc != "" {
			out = append(out, rdf.Triple{Subject: subj, Predicate: rdf.Comment, Object: rdf.Lit(item.Desc)})
		}
		if item.Type != "" {
			t, err := s.ResolveName(item.Type)
			if err != nil {
				return nil, fmt.Errorf("解析 type %q: %w", item.Type, err)
			}
			out = append(out, rdf.Triple{Subject: subj, Predicate: rdf.Type, Object: t})
		}
		return out, nil
	}

	// 模式 2：关系三元组（subject/pred/object）
	if item.Subject != "" && item.Predicate != "" && item.Object != "" {
		subj, err := s.ResolveName(item.Subject)
		if err != nil {
			return nil, fmt.Errorf("解析 subject: %w", err)
		}
		pred, err := s.ResolveName(item.Predicate)
		if err != nil {
			return nil, fmt.Errorf("解析 pred: %w", err)
		}
		var obj rdf.Term
		if item.Datatype != "" {
			obj = rdf.TypedLit(item.Object, xsdIRI(item.Datatype))
		} else {
			// 默认当 IRI 实体（关系），失败则当字面量
			o, err := s.ResolveName(item.Object)
			if err != nil {
				obj = rdf.Lit(item.Object)
			} else {
				obj = o
			}
		}
		return []rdf.Triple{{Subject: subj, Predicate: pred, Object: obj}}, nil
	}

	return nil, fmt.Errorf("项既无 name 也无 subject/pred/object")
}

// xsdIRI 把简短类型名（dateTime/decimal/...）转成完整 XSD IRI。
func xsdIRI(short string) string {
	if full, ok := xsdShortcuts[short]; ok {
		return full
	}
	return short // 已是完整 IRI 或未知类型，原样返回
}

var xsdShortcuts = map[string]string{
	"string":   rdf.XSDString,
	"dateTime": "http://www.w3.org/2001/XMLSchema#dateTime",
	"date":     "http://www.w3.org/2001/XMLSchema#date",
	"decimal":  "http://www.w3.org/2001/XMLSchema#decimal",
	"integer":  rdf.XSDInteger,
	"boolean":  rdf.XSDBoolean,
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// resolveStore 是 store.Store 的最小子集（apply 用）。
type resolveStore interface {
	LocalIRI(string) rdf.Term
	ResolveName(string) (rdf.Term, error)
	Has(rdf.Triple) bool
	Add(rdf.Triple)
}

// splitKV 拆 "key=value"，返回 (key, value, ok)。
func splitKV(s string) (string, string, bool) {
	i := -1
	for j, c := range s {
		if c == '=' {
			i = j
			break
		}
	}
	if i < 0 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}

// autotypeLiteral 根据字符串内容自动选择合适的 xsd 类型。
//   - "2026-05-20" / "2026-05-20T10:30:00" → dateTime/date
//   - "7.28" → decimal
//   - "true" / "false" → boolean
//   - 其他 → plain string
func autotypeLiteral(v string) rdf.Term {
	// ISO 日期/日期时间
	if isISODate(v) {
		return rdf.TypedLit(v, "http://www.w3.org/2001/XMLSchema#dateTime")
	}
	if isISODateOnly(v) {
		return rdf.TypedLit(v, "http://www.w3.org/2001/XMLSchema#date")
	}
	// 布尔
	if v == "true" || v == "false" {
		return rdf.TypedLit(v, rdf.XSDBoolean)
	}
	// 数字
	if isDecimal(v) {
		return rdf.TypedLit(v, "http://www.w3.org/2001/XMLSchema#decimal")
	}
	return rdf.Lit(v)
}

func isISODate(v string) bool {
	// YYYY-MM-DDTHH:MM:SS
	if len(v) != 19 || v[4] != '-' || v[7] != '-' || v[10] != 'T' || v[13] != ':' || v[16] != ':' {
		return false
	}
	for i, c := range v {
		if i == 4 || i == 7 || i == 10 || i == 13 || i == 16 {
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isISODateOnly(v string) bool {
	if len(v) != 10 || v[4] != '-' || v[7] != '-' {
		return false
	}
	for i, c := range v {
		if i == 4 || i == 7 {
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isDecimal(v string) bool {
	if v == "" {
		return false
	}
	dotCount := 0
	for i, c := range v {
		if c == '.' {
			dotCount++
			if dotCount > 1 {
				return false
			}
			continue
		}
		if c == '-' && i == 0 {
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return dotCount == 1 // 严格要求有小数点（整数用 integer 更准，但简化为 decimal）
}
