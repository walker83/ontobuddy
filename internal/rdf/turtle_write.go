package rdf

import (
	"fmt"
	"sort"
	"strings"
)

// PrefixMap 维护前缀缩写 <-> 命名空间 URI 的映射，
// 用于 Turtle 序列化时把完整 IRI 缩写成 prefix:local 形式。
type PrefixMap map[string]string // prefix -> namespace base URI

// Turtle 语句分组字符。
const turtleIndent = "    "

// SerializeTurtle 把三元组按 Turtle 格式序列化到字符串。
// 相同 subject 的连续三元组会被合并为一条用 ; 分隔的语句，
// 相同 subject+predicate 的连续三元组用 , 分隔，输出更紧凑易读。
func SerializeTurtle(triples []Triple, prefixes PrefixMap) string {
	if len(triples) == 0 {
		// 仍然输出前缀声明，便于后续手写编辑。
		return formatPrefixes(prefixes) + "\n"
	}

	// 按 subject 分组，再按 predicate 分组。
	type predGroup struct {
		pred Term
		objs []Term
	}
	type subjGroup struct {
		subj  Term
		preds []predGroup
	}

	// 建立索引（保持稳定顺序）。
	subjOrder := []Term{}
	subjIdx := map[Term]int{}
	predOrder := map[Term][]Term{}           // subject -> predicate 顺序
	predObjSet := map[Term]map[Term][]Term{} // subject -> pred -> objs

	for _, t := range triples {
		si, ok := subjIdx[t.Subject]
		if !ok {
			si = len(subjOrder)
			subjIdx[t.Subject] = si
			subjOrder = append(subjOrder, t.Subject)
			predOrder[t.Subject] = nil
			predObjSet[t.Subject] = map[Term][]Term{}
		}
		_ = si

		// predicate 顺序
		found := false
		for _, p := range predOrder[t.Subject] {
			if p.Equal(t.Predicate) {
				found = true
				break
			}
		}
		if !found {
			predOrder[t.Subject] = append(predOrder[t.Subject], t.Predicate)
		}

		// object 去重
		objs := predObjSet[t.Subject][t.Predicate]
		dup := false
		for _, o := range objs {
			if o.Equal(t.Object) {
				dup = true
				break
			}
		}
		if !dup {
			predObjSet[t.Subject][t.Predicate] = append(objs, t.Object)
		}
	}

	// 排序 subject（让输出稳定：IRI 优先，按 Value 字母序）。
	sort.SliceStable(subjOrder, func(i, j int) bool {
		a, b := subjOrder[i], subjOrder[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Value < b.Value
	})

	var b strings.Builder
	b.WriteString(formatPrefixes(prefixes))
	b.WriteString("\n")

	for _, s := range subjOrder {
		writeTermShort(&b, s, prefixes)
		b.WriteByte(' ')
		for pi, p := range predOrder[s] {
			if pi > 0 {
				b.WriteString(" ;\n")
				b.WriteString(turtleIndent)
			}
			writeTermShort(&b, p, prefixes)
			b.WriteByte(' ')
			objs := predObjSet[s][p]
			for oi, o := range objs {
				if oi > 0 {
					b.WriteString(", ")
				}
				writeTermShort(&b, o, prefixes)
			}
		}
		b.WriteString(" .\n")
	}

	return b.String()
}

// formatPrefixes 输出 @prefix 声明，按 prefix 字母序排列。
func formatPrefixes(prefixes PrefixMap) string {
	if len(prefixes) == 0 {
		return ""
	}
	keys := make([]string, 0, len(prefixes))
	for k := range prefixes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "@prefix %s: <%s> .\n", k, prefixes[k])
	}
	return b.String()
}

// writeTermShort 写出 Term，IRI 优先用前缀缩写。
func writeTermShort(b *strings.Builder, t Term, prefixes PrefixMap) {
	switch t.Kind {
	case KindIRI:
		// 尝试用已知前缀缩写。
		for prefix, base := range prefixes {
			if strings.HasPrefix(t.Value, base) {
				local := strings.TrimPrefix(t.Value, base)
				if isSafeLocalName(local) {
					fmt.Fprintf(b, "%s:%s", prefix, local)
					return
				}
			}
		}
		fmt.Fprintf(b, "<%s>", t.Value)
	case KindBlank:
		fmt.Fprintf(b, "_:%s", t.Value)
	case KindLiteral:
		b.WriteString(t.String())
	}
}

// isSafeLocalName 判断 local name 是否可作为 Turtle 的 PQN/缩写后缀。
// 简化判定：非空、不以 '.' 开头（与语句分隔符冲突）、不含空白与特殊字符。
func isSafeLocalName(s string) bool {
	if s == "" || s[0] == '.' {
		return false
	}
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '/' || r == '#' || r == ':' {
			return false
		}
	}
	return true
}
