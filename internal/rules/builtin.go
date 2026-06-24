package rules

import (
	"github.com/walker/myonto/internal/rdf"
)

// BuiltinFunc 是内置规则的执行函数签名。
// 返回本轮推导出的新三元组。
type BuiltinFunc func(known []rdf.Triple, ctx *Context) []rdf.Triple

// Context 为内置规则提供推理上下文。
type Context struct {
	Transitive map[rdf.Term]bool      // 声明为 owl:TransitiveProperty 的谓词
	Symmetric  map[rdf.Term]bool      // 声明为 owl:SymmetricProperty 的谓词
	Inverses   map[rdf.Term]rdf.Term  // owl:inverseOf 映射
}

// builtinRegistry 注册所有内置规则的实现。
var builtinRegistry = map[string]BuiltinFunc{
	"transitive-property": builtinTransitiveProperty,
	"symmetric-property":  builtinSymmetricProperty,
	"inverse-of":          builtinInverseOf,
	"subclass-transitive": builtinSubClassOfTransitive,
	"type-inheritance":    builtinTypeInheritance,
	"subproperty-transitive": builtinSubPropertyOfTransitive,
	"property-inheritance":   builtinPropertyInheritance,
	"domain":              builtinDomain,
	"range":               builtinRange,
}

// GetBuiltinFunc 返回指定 ID 的内置规则函数。
func GetBuiltinFunc(id string) (BuiltinFunc, bool) {
	f, ok := builtinRegistry[id]
	return f, ok
}

// --- 内置规则实现 ---

func builtinSubClassOfTransitive(known []rdf.Triple, _ *Context) []rdf.Triple {
	parents := map[rdf.Term][]rdf.Term{}
	for _, t := range known {
		if t.Predicate.Equal(rdf.SubClassOf) {
			parents[t.Subject] = append(parents[t.Subject], t.Object)
		}
	}
	var out []rdf.Triple
	for a, bs := range parents {
		for _, b := range bs {
			for _, c := range parents[b] {
				if a.Equal(c) {
					continue
				}
				out = append(out, rdf.Triple{Subject: a, Predicate: rdf.SubClassOf, Object: c})
			}
		}
	}
	return out
}

func builtinTypeInheritance(known []rdf.Triple, _ *Context) []rdf.Triple {
	parents := map[rdf.Term][]rdf.Term{}
	for _, t := range known {
		if t.Predicate.Equal(rdf.SubClassOf) {
			parents[t.Subject] = append(parents[t.Subject], t.Object)
		}
	}
	var out []rdf.Triple
	for _, t := range known {
		if !t.Predicate.Equal(rdf.Type) {
			continue
		}
		if t.Object.Equal(rdf.Class) || t.Object.Equal(rdf.OwlClass) {
			continue
		}
		for _, sup := range parents[t.Object] {
			out = append(out, rdf.Triple{Subject: t.Subject, Predicate: rdf.Type, Object: sup})
		}
	}
	return out
}

func builtinSubPropertyOfTransitive(known []rdf.Triple, _ *Context) []rdf.Triple {
	parents := map[rdf.Term][]rdf.Term{}
	for _, t := range known {
		if t.Predicate.Equal(rdf.SubPropertyOf) {
			parents[t.Subject] = append(parents[t.Subject], t.Object)
		}
	}
	var out []rdf.Triple
	for a, bs := range parents {
		for _, b := range bs {
			for _, c := range parents[b] {
				if a.Equal(c) {
					continue
				}
				out = append(out, rdf.Triple{Subject: a, Predicate: rdf.SubPropertyOf, Object: c})
			}
		}
	}
	return out
}

func builtinPropertyInheritance(known []rdf.Triple, _ *Context) []rdf.Triple {
	parents := map[rdf.Term][]rdf.Term{}
	for _, t := range known {
		if t.Predicate.Equal(rdf.SubPropertyOf) {
			parents[t.Subject] = append(parents[t.Subject], t.Object)
		}
	}
	var out []rdf.Triple
	for _, t := range known {
		if t.Predicate.Kind != rdf.KindIRI {
			continue
		}
		if t.Predicate.Equal(rdf.Type) || t.Predicate.Equal(rdf.Label) ||
			t.Predicate.Equal(rdf.Comment) || t.Predicate.Equal(rdf.SubClassOf) ||
			t.Predicate.Equal(rdf.SubPropertyOf) {
			continue
		}
		for _, sup := range parents[t.Predicate] {
			out = append(out, rdf.Triple{Subject: t.Subject, Predicate: sup, Object: t.Object})
		}
	}
	return out
}

func builtinTransitiveProperty(known []rdf.Triple, ctx *Context) []rdf.Triple {
	byPred := map[rdf.Term]map[rdf.Term][]rdf.Term{}
	for _, t := range known {
		if !ctx.Transitive[t.Predicate] {
			continue
		}
		if byPred[t.Predicate] == nil {
			byPred[t.Predicate] = map[rdf.Term][]rdf.Term{}
		}
		byPred[t.Predicate][t.Subject] = append(byPred[t.Predicate][t.Subject], t.Object)
	}
	var out []rdf.Triple
	for p, subjMap := range byPred {
		for a, bs := range subjMap {
			visited := map[rdf.Term]bool{}
			queue := append([]rdf.Term(nil), bs...)
			for len(queue) > 0 {
				b := queue[0]
				queue = queue[1:]
				if visited[b] {
					continue
				}
				visited[b] = true
				if a != b {
					out = append(out, rdf.Triple{Subject: a, Predicate: p, Object: b})
				}
				for _, c := range subjMap[b] {
					if !visited[c] {
						queue = append(queue, c)
					}
				}
			}
		}
	}
	return out
}

func builtinSymmetricProperty(known []rdf.Triple, ctx *Context) []rdf.Triple {
	var out []rdf.Triple
	for _, t := range known {
		if ctx.Symmetric[t.Predicate] {
			out = append(out, rdf.Triple{Subject: t.Object, Predicate: t.Predicate, Object: t.Subject})
		}
	}
	return out
}

func builtinInverseOf(known []rdf.Triple, ctx *Context) []rdf.Triple {
	var out []rdf.Triple
	for _, t := range known {
		if inv, ok := ctx.Inverses[t.Predicate]; ok {
			out = append(out, rdf.Triple{Subject: t.Object, Predicate: inv, Object: t.Subject})
		}
	}
	return out
}

func builtinDomain(known []rdf.Triple, _ *Context) []rdf.Triple {
	domainOf := map[rdf.Term][]rdf.Term{}
	for _, t := range known {
		if t.Predicate.Equal(rdf.Domain) {
			domainOf[t.Subject] = append(domainOf[t.Subject], t.Object)
		}
	}
	if len(domainOf) == 0 {
		return nil
	}
	var out []rdf.Triple
	for _, t := range known {
		if isMetaPredicate(t.Predicate) {
			continue
		}
		if t.Predicate.Equal(rdf.Type) || t.Predicate.Equal(rdf.Domain) || t.Predicate.Equal(rdf.Range) {
			continue
		}
		for _, c := range domainOf[t.Predicate] {
			out = append(out, rdf.Triple{Subject: t.Subject, Predicate: rdf.Type, Object: c})
		}
	}
	return out
}

func builtinRange(known []rdf.Triple, _ *Context) []rdf.Triple {
	rangeOf := map[rdf.Term][]rdf.Term{}
	for _, t := range known {
		if t.Predicate.Equal(rdf.Range) {
			rangeOf[t.Subject] = append(rangeOf[t.Subject], t.Object)
		}
	}
	if len(rangeOf) == 0 {
		return nil
	}
	var out []rdf.Triple
	for _, t := range known {
		if isMetaPredicate(t.Predicate) {
			continue
		}
		if t.Predicate.Equal(rdf.Type) || t.Predicate.Equal(rdf.Domain) || t.Predicate.Equal(rdf.Range) {
			continue
		}
		if t.Object.Kind != rdf.KindIRI {
			continue
		}
		for _, c := range rangeOf[t.Predicate] {
			out = append(out, rdf.Triple{Subject: t.Object, Predicate: rdf.Type, Object: c})
		}
	}
	return out
}

func isMetaPredicate(p rdf.Term) bool {
	return p.Equal(rdf.Label) || p.Equal(rdf.Comment) ||
		p.Equal(rdf.SubClassOf) || p.Equal(rdf.SubPropertyOf) ||
		p.Equal(rdf.DisjointWith) || p.Equal(rdf.EquivalentClass) ||
		p.Equal(rdf.InverseOf)
}
