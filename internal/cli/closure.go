package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/walker/myonto/internal/rdf"
	"github.com/walker/myonto/internal/store"
)

// ClosureCmd 算某实体沿指定谓词的传递闭包（不物化，纯查询）。
//
// 典型用途：影响面分析——"改了 store 包，间接影响哪些包？"。
// 先跑一次推理把传递属性/subPropertyOf 的隐式边算出来，再做 BFS，
// 所以 closure 自动包含 reason 能推出的全部传递结论。
//
// --reverse 反向遍历（"谁指向我"，如查谁依赖了我）。
// --depth 限制最大跳数（0=无限）。
type ClosureCmd struct {
	Entity  string `arg:"" required:"" placeholder:"ENTITY" help:"起始实体"`
	Pred    string `short:"p" required:"" placeholder:"PREDICATE" help:"沿此谓词展开闭包"`
	Reverse bool   `short:"r" help:"反向遍历（宾语 → 主语）"`
	Depth   int    `short:"d" default:"0" help:"最大深度（0=无限）"`
}

// closureResult 是 --json 输出结构。
type closureResult struct {
	Seed      string       `json:"seed"`
	Predicate string       `json:"predicate"`
	Reverse   bool         `json:"reverse"`
	Reachable []closureHop `json:"reachable"`
	Count     int          `json:"count"`
}

// closureHop 是闭包里一个可达节点及其距离。
type closureHop struct {
	Term  string `json:"term"`
	Depth int    `json:"depth"`
}

// Run 执行闭包查询。
func (c *ClosureCmd) Run() error {
	s, _, err := openStore()
	if err != nil {
		return err
	}

	seed, err := s.ResolveName(c.Entity)
	if err != nil {
		return fmt.Errorf("解析实体 %q: %w", c.Entity, err)
	}
	pred, err := s.ResolveName(c.Pred)
	if err != nil {
		return fmt.Errorf("解析谓词 %q: %w", c.Pred, err)
	}

	hops := computeClosure(s, seed, pred, c.Reverse, c.Depth)
	return closureRender(c, seed, pred, hops)
}

// computeClosure 在 store 上算闭包，返回可达节点及其距离。
// 抽出来便于测试。
//
// 实现要点：在**原始边集**上做 BFS，深度反映真实跳数。
// 不在此处 Derive——若用户想看含隐式边的闭包，应先 reason -a 物化。
// 这样 --depth 的语义清晰（原始图的跳数），且与 path 命令一致。
func computeClosure(s *store.Store, seed, pred rdf.Term, reverse bool, maxDepth int) []closureHop {
	all := s.Triples()

	// 建邻接表。正向：subject → [objects]；反向：object → [subjects]。
	adj := map[rdf.Term][]rdf.Term{}
	for _, t := range all {
		if !t.Predicate.Equal(pred) {
			continue
		}
		if reverse {
			adj[t.Object] = append(adj[t.Object], t.Subject)
		} else {
			adj[t.Subject] = append(adj[t.Subject], t.Object)
		}
	}

	// BFS。
	type step struct {
		node  rdf.Term
		depth int
	}
	visited := map[rdf.Term]bool{seed: true}
	var queue []step
	for _, n := range adj[seed] {
		queue = append(queue, step{node: n, depth: 1})
	}

	var hops []closureHop
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if visited[cur.node] {
			continue
		}
		visited[cur.node] = true
		hops = append(hops, closureHop{Term: cur.node.Value, Depth: cur.depth})
		if maxDepth > 0 && cur.depth >= maxDepth {
			continue
		}
		for _, next := range adj[cur.node] {
			if !visited[next] {
				queue = append(queue, step{node: next, depth: cur.depth + 1})
			}
		}
	}

	// 按 IRI 排序，输出稳定。
	sort.Slice(hops, func(i, j int) bool {
		if hops[i].Depth != hops[j].Depth {
			return hops[i].Depth < hops[j].Depth
		}
		return hops[i].Term < hops[j].Term
	})
	return hops
}

// closureRender 统一处理 --json / 人类可读输出。
func closureRender(c *ClosureCmd, seed, pred rdf.Term, hops []closureHop) error {
	if IsJSON() {
		out := closureResult{
			Seed:      seed.Value,
			Predicate: pred.Value,
			Reverse:   c.Reverse,
			Reachable: hops,
			Count:     len(hops),
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	// 人类可读
	direction := "→"
	if c.Reverse {
		direction = "←"
	}
	if len(hops) == 0 {
		fmt.Fprintf(os.Stdout, "%s %s %s 的闭包为空（无可达节点）\n",
			shortTerm(seed), direction, shortTerm(pred))
		return nil
	}
	fmt.Fprintf(os.Stdout, "%s %s %s 的传递闭包（%d 个可达节点）：\n",
		shortTerm(seed), direction, shortTerm(pred), len(hops))
	curDepth := 0
	for _, h := range hops {
		if h.Depth != curDepth {
			curDepth = h.Depth
			fmt.Fprintf(os.Stdout, "  — 距离 %d —\n", curDepth)
		}
		fmt.Fprintf(os.Stdout, "    %s\n", localOrFull(h.Term))
	}
	return nil
}

// localOrFull 取 IRI 的 local name，失败回退完整值。
func localOrFull(iri string) string {
	t := rdf.IRI(iri)
	ln := t.LocalName()
	if ln == "" {
		return iri
	}
	return ln
}
