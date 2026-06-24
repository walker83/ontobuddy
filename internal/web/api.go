package web

import (
	"strings"

	"github.com/walker/myonto/internal/rdf"
	"github.com/walker/myonto/internal/store"
)

// GraphNode 是图中的一个节点。
type GraphNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Group string `json:"group"` // "class", "individual", "property"
}

// GraphEdge 是图中的一条边。
type GraphEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
}

// buildGraph 从 store 中提取图数据。
func buildGraph(s *store.Store) ([]GraphNode, []GraphEdge) {
	triples := s.Triples()
	nodeSet := map[string]GraphNode{}
	var edges []GraphEdge

	for _, t := range triples {
		subjID := termID(t.Subject)
		objID := termID(t.Object)
		predLabel := t.Predicate.LocalName()

		// 确定节点分组
		subjGroup := classifyTerm(t.Subject, t.Predicate)
		objGroup := classifyTerm(t.Object, rdf.Term{})

		if _, ok := nodeSet[subjID]; !ok {
			nodeSet[subjID] = GraphNode{ID: subjID, Label: termLabel(t.Subject), Group: subjGroup}
		}
		if _, ok := nodeSet[objID]; !ok {
			nodeSet[objID] = GraphNode{ID: objID, Label: termLabel(t.Object), Group: objGroup}
		}

		edges = append(edges, GraphEdge{From: subjID, To: objID, Label: predLabel})
	}

	nodes := make([]GraphNode, 0, len(nodeSet))
	for _, n := range nodeSet {
		nodes = append(nodes, n)
	}
	return nodes, edges
}

// queryTriples 按模式查询三元组。
func queryTriples(s *store.Store, subject, predicate, object string) []rdf.Triple {
	// 如果有精确匹配，使用 store 的 Query 方法
	if subject != "" || predicate != "" || object != "" {
		pattern := rdf.Triple{}
		if subject != "" {
			pattern.Subject = rdf.IRI(subject)
		}
		if predicate != "" {
			pattern.Predicate = rdf.IRI(predicate)
		}
		if object != "" {
			pattern.Object = rdf.IRI(object)
		}
		return s.Query(pattern)
	}
	// 无过滤条件，返回全部
	return s.Triples()
}

// searchTriples 模糊搜索三元组。
func searchTriples(s *store.Store, q string) []rdf.Triple {
	if q == "" {
		return s.Triples()
	}
	q = strings.ToLower(q)
	var result []rdf.Triple
	for _, t := range s.Triples() {
		if strings.Contains(strings.ToLower(t.Subject.Value), q) ||
			strings.Contains(strings.ToLower(t.Predicate.Value), q) ||
			strings.Contains(strings.ToLower(t.Object.Value), q) {
			result = append(result, t)
		}
	}
	return result
}

func termID(t rdf.Term) string {
	if t.LocalName() != "" {
		return t.LocalName()
	}
	return t.Value
}

func termLabel(t rdf.Term) string {
	if l := t.LocalName(); l != "" {
		return l
	}
	return t.Value
}

func classifyTerm(t rdf.Term, pred rdf.Term) string {
	// 字面量
	if t.Kind == rdf.KindLiteral {
		return "literal"
	}
	// 通过谓词判断
	if pred.Equal(rdf.Type) {
		return "class"
	}
	if pred.Equal(rdf.SubClassOf) || pred.Equal(rdf.Domain) || pred.Equal(rdf.Range) {
		return "class"
	}
	return "individual"
}
