package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/walker/myonto/internal/rdf"
	"github.com/walker/myonto/internal/reasoning"
	"github.com/walker/myonto/internal/store"
)

// PathCmd 找两个实体间的最短路径（BFS）。
//
// 典型用途：解释性查询——"A 和 B 是怎么关联上的？"。
// 走所有 IRI 宾语的边（不限谓词），或用 --pred 限定只走某谓词。
// 先跑推理合并隐式边，所以路径可包含 reason 推出的关系。
//
// 找不到路径时退出码 1（便于脚本判断连通性）。
type PathCmd struct {
	From string `arg:"" required:"" placeholder:"FROM" help:"起点实体"`
	To   string `arg:"" required:"" placeholder:"TO" help:"终点实体"`
	Pred string `short:"p" placeholder:"PREDICATE" help:"只走此谓词的边（默认所有 IRI 边）"`
}

// pathResult 是 --json 输出结构。
type pathResult struct {
	From   string       `json:"from"`
	To     string       `json:"to"`
	Found  bool         `json:"found"`
	Length int          `json:"length"`
	Path   []pathTriple `json:"path,omitempty"`
}

// pathTriple 是路径上的一条有向边。
type pathTriple struct {
	Subject   string         `json:"subject"`
	Predicate string         `json:"predicate"`
	Object    map[string]any `json:"object"`
}

// Run 执行最短路径查询。
func (c *PathCmd) Run() error {
	s, _, err := openStore()
	if err != nil {
		return err
	}
	from, err := s.ResolveName(c.From)
	if err != nil {
		return fmt.Errorf("解析起点 %q: %w", c.From, err)
	}
	to, err := s.ResolveName(c.To)
	if err != nil {
		return fmt.Errorf("解析终点 %q: %w", c.To, err)
	}
	var predFilter rdf.Term
	hasPredFilter := false
	if c.Pred != "" {
		predFilter, err = s.ResolveName(c.Pred)
		if err != nil {
			return fmt.Errorf("解析谓词 %q: %w", c.Pred, err)
		}
		hasPredFilter = true
	}

	edges := findPath(s, from, to, predFilter, hasPredFilter)
	found := len(edges) > 0 || from.Equal(to)
	return pathRender(c, from, to, edges, found)
}

// findPath 用 BFS 找 from→to 的最短路径，返回路径上的有向边序列。
// 抽出来便于测试。from==to 时返回空边（视为找到，长度 0）。
//
// 邻接表只含 IRI 宾语的边（字面量不是节点，不参与路径）。
// hasPredFilter 为 true 时用 predFilter 限定谓词。
func findPath(s *store.Store, from, to, predFilter rdf.Term, hasPredFilter bool) []rdf.Triple {
	if from.Equal(to) {
		return nil
	}
	// 完整边集 = 原始 + 推导。
	all := s.Triples()
	derived := reasoning.NewReasoner(all).Derive()
	edges := append([]rdf.Triple{}, all...)
	edges = append(edges, derived...)

	// 建邻接表。
	adj := map[rdf.Term][]rdf.Triple{}
	for _, t := range edges {
		if t.Object.Kind != rdf.KindIRI {
			continue // 字面量宾语不是节点
		}
		if hasPredFilter && !t.Predicate.Equal(predFilter) {
			continue
		}
		adj[t.Subject] = append(adj[t.Subject], t)
	}

	// BFS，记录前驱边用于回溯。
	type state struct {
		node    rdf.Term
		viaEdge rdf.Triple // 到达此节点的边
	}
	visited := map[rdf.Term]rdf.Triple{from: {}} // from 的前驱边为零值
	queue := []state{{node: from}}
	goalFound := false

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, e := range adj[cur.node] {
			next := e.Object
			if _, seen := visited[next]; seen {
				continue
			}
			visited[next] = e
			if next.Equal(to) {
				goalFound = true
				queue = nil // 提前结束
				break
			}
			queue = append(queue, state{node: next, viaEdge: e})
		}
	}

	if !goalFound {
		return nil
	}

	// 回溯重建路径。
	var path []rdf.Triple
	cur := to
	for !cur.Equal(from) {
		e := visited[cur]
		path = append(path, e)
		cur = e.Subject
	}
	// 反转成 from→to 顺序。
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// pathRender 统一处理 --json / 人类可读输出。
func pathRender(c *PathCmd, from, to rdf.Term, edges []rdf.Triple, found bool) error {
	if IsJSON() {
		out := pathResult{
			From:   from.Value,
			To:     to.Value,
			Found:  found,
			Length: len(edges),
		}
		for _, e := range edges {
			out.Path = append(out.Path, pathTriple{
				Subject:   e.Subject.Value,
				Predicate: e.Predicate.Value,
				Object:    termToJSON(e.Object),
			})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		if !found {
			exitWith(1)
		}
		return nil
	}

	// 人类可读
	if !found {
		fmt.Fprintf(os.Stdout, "✗ %s 与 %s 之间无路径\n", shortTerm(from), shortTerm(to))
		exitWith(1)
	}
	if len(edges) == 0 {
		fmt.Fprintf(os.Stdout, "%s 与 %s 是同一节点（距离 0）\n", shortTerm(from), shortTerm(to))
		return nil
	}
	fmt.Fprintf(os.Stdout, "%s → %s 最短路径（%d 跳）：\n", shortTerm(from), shortTerm(to), len(edges))
	for i, e := range edges {
		fmt.Fprintf(os.Stdout, "  %d. %s\n", i+1, formatTriple(e))
	}
	return nil
}
