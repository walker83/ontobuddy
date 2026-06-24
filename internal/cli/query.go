package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/walker/myonto/internal/rdf"
	"github.com/walker/myonto/internal/reasoning"
	"github.com/walker/myonto/internal/store"
)

// QueryCmd 轻量查询引擎：三元组模式匹配 + GROUP BY / COUNT / Top-N。
//
// 设计为 SPARQL 子集的声明式 CLI（避免完整 SPARQL parser 负担）：
//
//	myonto query --where '?s rdf:type ex:Person' --count
//	myonto query --where '?s ex:bornIn ?o' --group-by '?o' --count --top 5
//	myonto query --where '?s ex:knows ?o' --where '?o ex:knows ex:carol'
//
// 关键设计：先跑一次推理把隐式知识物化进可见三元组集，
// 让聚合自动包含 subClassOf/domain/range 等推出的结论。
// 这是本体驱动数据分析的核心价值——统计的是"语义上成立的事实"而非仅显式断言。
//
// 变量语法：?name。多个 --where 共享变量时做 JOIN。
type QueryCmd struct {
	Where    []string `short:"w" help:"三元组模式（'?s P ?o' 形式，?var 为变量，多 --where 做 JOIN）"`
	GroupBy  string   `short:"g" placeholder:"VAR" help:"按某变量分组"`
	Count    bool     `short:"c" help:"COUNT 每组元素数"`
	SortDesc bool     `help:"按 count 降序（默认降序）；不分组时按变量升序"`
	Top      int      `short:"n" default:"0" help:"Top-N（0=全部）"`
	Distinct bool     `short:"d" help:"结果去重"`
}

// binding 是一组变量→Term 的绑定。
type binding map[string]rdf.Term

// queryResult 是 --json 输出结构。
type queryResult struct {
	Patterns []string     `json:"patterns"`
	GroupBy  string       `json:"group_by,omitempty"`
	Count    bool         `json:"count"`
	Results  []queryRow   `json:"results"`
	Total    int          `json:"total"`
}

// queryRow 是一行结果：分组键 + 计数。
type queryRow struct {
	Key   string `json:"key"`
	Count int    `json:"count,omitempty"`
}

// Run 执行查询。
func (c *QueryCmd) Run() error {
	s, _, err := openStore()
	if err != nil {
		return err
	}
	rows, patterns, err := runQuery(s, c)
	if err != nil {
		return err
	}
	return queryRender(c, rows, patterns)
}

// runQuery 执行查询逻辑，返回结果行。抽出来便于测试。
func runQuery(s *store.Store, c *QueryCmd) ([]queryRow, []string, error) {
	if len(c.Where) == 0 {
		return nil, nil, fmt.Errorf("至少需要一个 --where 模式")
	}

	// 解析模式。
	patterns := make([]triplePattern, len(c.Where))
	for i, w := range c.Where {
		p, err := parsePattern(w, s)
		if err != nil {
			return nil, nil, fmt.Errorf("模式 %d: %w", i+1, err)
		}
		patterns[i] = p
	}

	// 完整事实集 = 原始 + 推导。聚合基于此，包含隐式知识。
	all := s.Triples()
	derived := reasoning.NewReasoner(all).Derive()
	facts := append([]rdf.Triple{}, all...)
	facts = append(facts, derived...)

	// 匹配所有模式（JOIN）。
	bindings := matchPatterns(facts, patterns)

	// 处理 --distinct（在分组前去重绑定）。
	if c.Distinct && c.GroupBy == "" {
		bindings = dedupBindings(bindings)
	}

	// 分组聚合。
	var rows []queryRow
	if c.GroupBy != "" {
		groupVar := strings.TrimPrefix(c.GroupBy, "?")
		rows = groupAndCount(bindings, groupVar)
	} else if c.Count {
		// 无分组但要 count：统计总绑定数。
		rows = []queryRow{{Key: "*", Count: len(bindings)}}
	} else {
		// 纯投影：输出每个绑定的第一个变量值。
		rows = projectBindings(bindings, patterns)
	}

	// 排序。
	sortRows(rows, c.SortDesc, c.GroupBy != "")

	// Top-N。
	if c.Top > 0 && c.Top < len(rows) {
		rows = rows[:c.Top]
	}

	patternStrs := make([]string, len(c.Where))
	copy(patternStrs, c.Where)
	return rows, patternStrs, nil
}

// triplePattern 是一个解析后的三元组模式，每个位置要么是变量要么是常量 Term。
type triplePattern struct {
	SubjVar, PredVar, ObjVar string           // 变量名（空表示该位置是常量）
	SubjTerm, PredTerm, ObjTerm rdf.Term      // 常量值（当对应变量名为空时有效）
}

// parsePattern 把 "?s P ?o" 形式的字符串解析为 triplePattern。
// 支持：?var（变量）、IRI（<...> 或 prefix:local）、字面量（"..."）。
// 用 store 解析 prefix:local 形式。
func parsePattern(s string, st *store.Store) (triplePattern, error) {
	parts := strings.Fields(strings.TrimSpace(s))
	if len(parts) != 3 {
		return triplePattern{}, fmt.Errorf("模式应为 'S P O' 三段，got %d 段: %q", len(parts), s)
	}
	var p triplePattern
	var err error
	p.SubjVar, p.SubjTerm, err = parseTerm(parts[0], st)
	if err != nil {
		return p, fmt.Errorf("主语 %q: %w", parts[0], err)
	}
	p.PredVar, p.PredTerm, err = parseTerm(parts[1], st)
	if err != nil {
		return p, fmt.Errorf("谓词 %q: %w", parts[1], err)
	}
	p.ObjVar, p.ObjTerm, err = parseTerm(parts[2], st)
	if err != nil {
		return p, fmt.Errorf("宾语 %q: %w", parts[2], err)
	}
	return p, nil
}

// parseTerm 解析单个 term：变量、IRI 或字面量。
// 返回 (变量名, 常量Term)；变量名为空时 Term 有效。
func parseTerm(tok string, st *store.Store) (varName string, term rdf.Term, err error) {
	if strings.HasPrefix(tok, "?") {
		return tok[1:], rdf.Term{}, nil
	}
	// Turtle 关键字 a = rdf:type
	if tok == "a" {
		return "", rdf.Type, nil
	}
	// 字面量
	if strings.HasPrefix(tok, `"`) && strings.HasSuffix(tok, `"`) {
		return "", rdf.Lit(tok[1 : len(tok)-1]), nil
	}
	// IRI：<...> 完整形式
	if strings.HasPrefix(tok, "<") && strings.HasSuffix(tok, ">") {
		return "", rdf.IRI(tok[1 : len(tok)-1]), nil
	}
	// prefix:local 或裸 local name：用 store 解析
	term, err = st.ResolveName(tok)
	if err != nil {
		return "", rdf.Term{}, err
	}
	return "", term, nil
}

// matchPatterns 对多个模式做 JOIN，返回所有满足的变量绑定。
func matchPatterns(facts []rdf.Triple, patterns []triplePattern) []binding {
	// 从第一个模式开始，逐个 JOIN。
	var results []binding
	// 初始：第一个模式的所有匹配。
	results = matchOne(facts, patterns[0], nil)

	for i := 1; i < len(patterns); i++ {
		var next []binding
		for _, b := range results {
			matches := matchOne(facts, patterns[i], b)
			next = append(next, matches...)
		}
		results = next
	}
	return results
}

// matchOne 用单模式匹配 facts，与已有绑定 consistent-merge。
// existing 为 nil 时是第一个模式。
func matchOne(facts []rdf.Triple, p triplePattern, existing binding) []binding {
	var out []binding
	for _, t := range facts {
		b := mergeMatch(t, p, existing)
		if b != nil {
			out = append(out, b)
		}
	}
	return out
}

// mergeMatch 检查三元组 t 是否匹配模式 p（考虑已有绑定 existing）。
// 匹配则返回合并后的新 binding（拷贝），不匹配返回 nil。
func mergeMatch(t rdf.Triple, p triplePattern, existing binding) binding {
	b := binding{}
	if existing != nil {
		for k, v := range existing {
			b[k] = v
		}
	}
	if !bindPosition(b, p.SubjVar, p.SubjTerm, t.Subject) {
		return nil
	}
	if !bindPosition(b, p.PredVar, p.PredTerm, t.Predicate) {
		return nil
	}
	if !bindPosition(b, p.ObjVar, p.ObjTerm, t.Object) {
		return nil
	}
	return b
}

// bindPosition 把一个位置（变量或常量）绑定到实际 term。
// 变量：若已绑定则检查一致性，否则绑定；常量：检查相等。
func bindPosition(b binding, varName string, constTerm, actual rdf.Term) bool {
	if varName != "" {
		if existing, ok := b[varName]; ok {
			return existing.Equal(actual)
		}
		b[varName] = actual
		return true
	}
	return constTerm.Equal(actual)
}

// dedupBindings 按完整绑定去重。
func dedupBindings(bindings []binding) []binding {
	seen := map[string]bool{}
	var out []binding
	for _, b := range bindings {
		k := bindingKey(b)
		if !seen[k] {
			seen[k] = true
			out = append(out, b)
		}
	}
	return out
}

func bindingKey(b binding) string {
	keys := make([]string, 0, len(b))
	for k := range b {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(b[k].String())
		sb.WriteByte(';')
	}
	return sb.String()
}

// groupAndCount 按 groupByVar 分组并计数。
func groupAndCount(bindings []binding, groupByVar string) []queryRow {
	counts := map[string]int{}
	order := []string{}
	for _, b := range bindings {
		term, ok := b[groupByVar]
		if !ok {
			continue
		}
		key := term.LocalName()
		if key == "" {
			key = term.Value
		}
		if _, seen := counts[key]; !seen {
			order = append(order, key)
		}
		counts[key]++
	}
	rows := make([]queryRow, 0, len(counts))
	for _, k := range order {
		rows = append(rows, queryRow{Key: k, Count: counts[k]})
	}
	return rows
}

// projectBindings 无分组时，输出每个绑定的第一个变量值。
func projectBindings(bindings []binding, patterns []triplePattern) []queryRow {
	// 找第一个模式里的变量。
	var firstVar string
	for _, p := range patterns {
		if p.SubjVar != "" {
			firstVar = p.SubjVar
			break
		}
		if p.PredVar != "" {
			firstVar = p.PredVar
			break
		}
		if p.ObjVar != "" {
			firstVar = p.ObjVar
			break
		}
	}
	rows := make([]queryRow, 0, len(bindings))
	for _, b := range bindings {
		term, ok := b[firstVar]
		if !ok {
			continue
		}
		key := term.LocalName()
		if key == "" {
			key = term.Value
		}
		rows = append(rows, queryRow{Key: key})
	}
	return rows
}

// sortRows 排序结果。分组模式默认按 count 降序；投影模式按 key 升序。
func sortRows(rows []queryRow, desc, grouped bool) {
	if grouped {
		// 按 count 降序，count 相同按 key 升序
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Count != rows[j].Count {
				if desc {
					return rows[i].Count > rows[j].Count
				}
				return rows[i].Count < rows[j].Count
			}
			return rows[i].Key < rows[j].Key
		})
		return
	}
	sort.Slice(rows, func(i, j int) bool {
		if desc {
			return rows[i].Key > rows[j].Key
		}
		return rows[i].Key < rows[j].Key
	})
}

// queryRender 统一处理 --json / 人类可读输出。
func queryRender(c *QueryCmd, rows []queryRow, patterns []string) error {
	if IsJSON() {
		out := queryResult{
			Patterns: patterns,
			GroupBy:  c.GroupBy,
			Count:    c.Count,
			Results:  rows,
			Total:    len(rows),
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	// 人类可读：表格形式。
	if len(rows) == 0 {
		fmt.Fprintln(os.Stdout, "（无匹配结果）")
		return nil
	}
	if c.GroupBy != "" || c.Count {
		// 分组/计数：key + count 两列。
		maxKey := 0
		for _, r := range rows {
			if len(r.Key) > maxKey {
				maxKey = len(r.Key)
			}
		}
		if maxKey < 4 {
			maxKey = 4
		}
		fmt.Fprintf(os.Stdout, "%-*s  %s\n", maxKey, c.GroupBy, "count")
		fmt.Fprintf(os.Stdout, "%s  %s\n", strings.Repeat("-", maxKey), "-----")
		for _, r := range rows {
			fmt.Fprintf(os.Stdout, "%-*s  %5d\n", maxKey, r.Key, r.Count)
		}
	} else {
		// 纯投影：单列。
		for _, r := range rows {
			fmt.Fprintln(os.Stdout, r.Key)
		}
	}
	fmt.Fprintf(os.Stdout, "\n共 %d 行\n", len(rows))
	return nil
}

// 保证 strconv 被引用（未来 count 格式化可能用到）。
var _ = strconv.Itoa
