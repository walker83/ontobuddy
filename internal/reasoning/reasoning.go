// Package reasoning 实现轻量的 RDFS/OWL 规则推理。
//
// 覆盖的规则（固定点闭包计算）：
//  1. rdfs:subClassOf 传递：A ⊑ B, B ⊑ C ⟹ A ⊑ C
//  2. 类型继承：x a A, A ⊑ B ⟹ x a B
//  3. rdfs:subPropertyOf 传递 + 继承
//  4. owl:TransitiveProperty：a P b, b P c ⟹ a P c（P 声明为 Transitive）
//  5. owl:SymmetricProperty：a P b ⟹ b P a
//  6. owl:inverseOf：a P b ⟹ b Q a（Q = inverse of P）
//  7. rdfs:domain：x P y, P domain C ⟹ x a C
//  8. rdfs:range：x P y, P range C ⟹ y a C（仅当 y 是 IRI；字面量不加类型）
//
// 注意：本项目**不实现完整的 OWL 2 DL 推理**（那需要 Java + HermiT/Pellet）。
// 这些是 OWL 2 RL 风格的「可规则化」子集，足以覆盖 90% 的个人本体场景。
package reasoning

import (
	"sort"

	"github.com/walker/myonto/internal/rdf"
)

// Reasoner 是对一组三元组跑规则推理的引擎。
// 构造时传入 base 三元组；Derive() 在 base 之上跑规则，输出新推导的三元组。
type Reasoner struct {
	base []rdf.Triple // 构造时的原始三元组（不动）

	// 谓词缓存（避免每次重新查询谓词类型）
	transitive map[rdf.Term]bool
	symmetric  map[rdf.Term]bool
	inverses   map[rdf.Term]rdf.Term
}

// NewReasoner 从 base 三元组中预扫描谓词类型信息。
func NewReasoner(base []rdf.Triple) *Reasoner {
	// 复制一份避免外部修改影响内部。
	cp := make([]rdf.Triple, len(base))
	copy(cp, base)
	r := &Reasoner{
		base:       cp,
		transitive: map[rdf.Term]bool{},
		symmetric:  map[rdf.Term]bool{},
		inverses:   map[rdf.Term]rdf.Term{},
	}
	for _, t := range cp {
		if t.Predicate.Equal(rdf.Type) {
			if t.Object.Equal(rdf.TransitiveProperty) {
				r.transitive[t.Subject] = true
			}
			if t.Object.Equal(rdf.SymmetricProperty) {
				r.symmetric[t.Subject] = true
			}
			if t.Object.Equal(rdf.OwlObjectProperty) {
				// 单独 ObjectProperty 不携带额外语义，但确认是属性即可。
			}
		}
		// inverseOf 是个二元属性：t.Subject 有一个属性 t.Object 是它的逆。
		if t.Predicate.Equal(rdf.InverseOf) {
			r.inverses[t.Subject] = t.Object
		}
	}
	return r
}

// Rule 是单条推理规则的接口。
type Rule interface {
	// Name 规则的人类可读名（用于输出哪条规则推导了这条三元组）。
	Name() string
	// Apply 在 known（已包含原始+已推导的三元组）上跑一次规则，
	// 返回本轮推导出的新三元组。
	Apply(known []rdf.Triple, r *Reasoner) []rdf.Triple
}

// Derive 在构造时传入的 base 之上跑全部规则直到不动点，
// 返回所有推导出的新三元组（不含原始三元组；与原始去重）。
func (r *Reasoner) Derive() []rdf.Triple {
	// 已知集合（key 用于去重）。
	known := make([]rdf.Triple, len(r.base))
	copy(known, r.base)
	knownSet := tripleKeySet(known)

	// 规则按顺序执行；每条规则每轮都可能产生新三元组，
	// 当所有规则都没有新输出时停止。
	allRules := []Rule{
		subClassOfTransitiveRule{},
		typeInheritanceRule{},
		subPropertyOfTransitiveRule{},
		propertyInheritanceRule{},
		transitivePropertyRule{r: r},
		symmetricPropertyRule{r: r},
		inverseOfRule{r: r},
		domainRule{},
		rangeRule{},
	}

	for {
		var newOnes []rdf.Triple
		for _, rule := range allRules {
			got := rule.Apply(known, r)
			for _, t := range got {
				k := tripleKey(t)
				if _, ok := knownSet[k]; ok {
					continue
				}
				knownSet[k] = struct{}{}
				known = append(known, t)
				newOnes = append(newOnes, t)
			}
		}
		if len(newOnes) == 0 {
			break
		}
	}

	// 过滤掉原始三元组。
	originalSet := tripleKeySet(r.base)
	out := make([]rdf.Triple, 0, len(known)-len(r.base))
	for _, t := range known {
		if _, ok := originalSet[tripleKey(t)]; ok {
			continue
		}
		out = append(out, t)
	}
	// 稳定排序：让输出可预测，便于测试。
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// 规则 1：subClassOf 传递闭包
type subClassOfTransitiveRule struct{}

func (subClassOfTransitiveRule) Name() string { return "subClassOf-transitive" }
func (subClassOfTransitiveRule) Apply(known []rdf.Triple, _ *Reasoner) []rdf.Triple {
	// 先建立父类索引：subject -> [super classes]
	parents := map[rdf.Term][]rdf.Term{}
	for _, t := range known {
		if t.Predicate.Equal(rdf.SubClassOf) {
			parents[t.Subject] = append(parents[t.Subject], t.Object)
		}
	}
	var out []rdf.Triple
	// 任意 A ⊑ B, B ⊑ C ⟹ A ⊑ C
	// 排除自环 A ⊑ A（无语义意义，但 `A ⊑ B, B ⊑ A` 这种等价环会推出自环，
	// 若不过滤会让输出含冗余三元组）。
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

// 规则 2：类型继承 —— x a A, A ⊑ B ⟹ x a B
type typeInheritanceRule struct{}

func (typeInheritanceRule) Name() string { return "type-inheritance" }
func (typeInheritanceRule) Apply(known []rdf.Triple, _ *Reasoner) []rdf.Triple {
	// 建立类的父类索引（复用规则 1 的中间结果）。
	parents := map[rdf.Term][]rdf.Term{}
	for _, t := range known {
		if t.Predicate.Equal(rdf.SubClassOf) {
			parents[t.Subject] = append(parents[t.Subject], t.Object)
		}
	}
	var out []rdf.Triple
	// x a A, A ⊑ B ⟹ x a B
	for _, t := range known {
		if !t.Predicate.Equal(rdf.Type) {
			continue
		}
		// 排除 rdf:type = rdfs:Class/owl:Class 的元数据。
		if t.Object.Equal(rdf.Class) || t.Object.Equal(rdf.OwlClass) {
			continue
		}
		for _, sup := range parents[t.Object] {
			out = append(out, rdf.Triple{Subject: t.Subject, Predicate: rdf.Type, Object: sup})
		}
	}
	return out
}

// 规则 3：subPropertyOf 传递闭包
type subPropertyOfTransitiveRule struct{}

func (subPropertyOfTransitiveRule) Name() string { return "subPropertyOf-transitive" }
func (subPropertyOfTransitiveRule) Apply(known []rdf.Triple, _ *Reasoner) []rdf.Triple {
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
					continue // 排除自环（见 subClassOfTransitiveRule 注释）
				}
				out = append(out, rdf.Triple{Subject: a, Predicate: rdf.SubPropertyOf, Object: c})
			}
		}
	}
	return out
}

// 规则 4：属性继承 —— a P b, P ⊑ Q ⟹ a Q b
type propertyInheritanceRule struct{}

func (propertyInheritanceRule) Name() string { return "property-inheritance" }
func (propertyInheritanceRule) Apply(known []rdf.Triple, _ *Reasoner) []rdf.Triple {
	parents := map[rdf.Term][]rdf.Term{}
	for _, t := range known {
		if t.Predicate.Equal(rdf.SubPropertyOf) {
			parents[t.Subject] = append(parents[t.Subject], t.Object)
		}
	}
	var out []rdf.Triple
	for _, t := range known {
		// 只对"是属性的 IRI 出现为谓词"的三元组做继承。
		// 这需要 t.Predicate 本身是个 IRI（不是字面量）。
		if t.Predicate.Kind != rdf.KindIRI {
			continue
		}
		// 跳过元谓词（如 rdf:type, rdfs:label）的继承——没意义。
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

// 规则 5：传递属性
type transitivePropertyRule struct{ r *Reasoner }

func (transitivePropertyRule) Name() string { return "owl:TransitiveProperty" }
func (transitivePropertyRule) Apply(known []rdf.Triple, r *Reasoner) []rdf.Triple {
	// 按谓词分组：P -> {a -> [b, ...]}
	byPred := map[rdf.Term]map[rdf.Term][]rdf.Term{}
	for _, t := range known {
		if !r.transitive[t.Predicate] {
			continue
		}
		if byPred[t.Predicate] == nil {
			byPred[t.Predicate] = map[rdf.Term][]rdf.Term{}
		}
		byPred[t.Predicate][t.Subject] = append(byPred[t.Predicate][t.Subject], t.Object)
	}
	var out []rdf.Triple
	for p, subjMap := range byPred {
		// bfs
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
				// a P b
				if a != b {
					out = append(out, rdf.Triple{Subject: a, Predicate: p, Object: b})
				}
				// b's P-successors 也都是 a 的
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

// 规则 6：对称属性
type symmetricPropertyRule struct{ r *Reasoner }

func (symmetricPropertyRule) Name() string { return "owl:SymmetricProperty" }
func (symmetricPropertyRule) Apply(known []rdf.Triple, r *Reasoner) []rdf.Triple {
	var out []rdf.Triple
	for _, t := range known {
		if r.symmetric[t.Predicate] {
			out = append(out, rdf.Triple{Subject: t.Object, Predicate: t.Predicate, Object: t.Subject})
		}
	}
	return out
}

// 规则 7：inverseOf
type inverseOfRule struct{ r *Reasoner }

func (inverseOfRule) Name() string { return "owl:inverseOf" }
func (inverseOfRule) Apply(known []rdf.Triple, r *Reasoner) []rdf.Triple {
	var out []rdf.Triple
	for _, t := range known {
		if inv, ok := r.inverses[t.Predicate]; ok {
			out = append(out, rdf.Triple{Subject: t.Object, Predicate: inv, Object: t.Subject})
		}
	}
	return out
}

// 规则 8：rdfs:domain —— x P y, P domain C ⟹ x a C
//
// 建谓词→域类索引，对每条以该谓词连接的三元组，把类型加给主语。
// 推出的 x a C 会喂回 typeInheritanceRule（x a C, C⊑D ⟹ x a D），
// 由不动点循环自动传播，无需特殊编排。
type domainRule struct{}

func (domainRule) Name() string { return "rdfs:domain" }
func (domainRule) Apply(known []rdf.Triple, _ *Reasoner) []rdf.Triple {
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
		// 跳过元谓词——给 rdfs:subClassOf 等加类型无意义，也避免给 schema 三元组的 subject 污染类型。
		if isMetaPredicate(t.Predicate) {
			continue
		}
		// 跳过 P 自身的 schema 声明（P a ObjectProperty / P domain C 这类）。
		if t.Predicate.Equal(rdf.Type) || t.Predicate.Equal(rdf.Domain) || t.Predicate.Equal(rdf.Range) {
			continue
		}
		for _, c := range domainOf[t.Predicate] {
			out = append(out, rdf.Triple{Subject: t.Subject, Predicate: rdf.Type, Object: c})
		}
	}
	return out
}

// 规则 9：rdfs:range —— x P y, P range C ⟹ y a C（仅当 y 是 IRI）
//
// 关键 soundness 守卫：字面量 object 不加类型（字面量没有类型身份，
// 给 "1643" 加 rdf:type Person 语义错误且会让去重失效）。
type rangeRule struct{}

func (rangeRule) Name() string { return "rdfs:range" }
func (rangeRule) Apply(known []rdf.Triple, _ *Reasoner) []rdf.Triple {
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
		// soundness：字面量 object 不加类型。
		if t.Object.Kind != rdf.KindIRI {
			continue
		}
		for _, c := range rangeOf[t.Predicate] {
			out = append(out, rdf.Triple{Subject: t.Object, Predicate: rdf.Type, Object: c})
		}
	}
	return out
}

// isMetaPredicate 判断一个谓词是否为 RDFS/OWL 内置元谓词。
// 对这些谓词的三元组不做 domain/range 推导——它们描述 schema 本身，
// 给其主语/宾语加用户类型会污染本体。
func isMetaPredicate(p rdf.Term) bool {
	return p.Equal(rdf.Label) || p.Equal(rdf.Comment) ||
		p.Equal(rdf.SubClassOf) || p.Equal(rdf.SubPropertyOf) ||
		p.Equal(rdf.DisjointWith) || p.Equal(rdf.EquivalentClass) ||
		p.Equal(rdf.InverseOf)
}

// --- 工具 ---

func tripleKey(t rdf.Triple) string { return t.String() }

func tripleKeySet(ts []rdf.Triple) map[string]struct{} {
	m := make(map[string]struct{}, len(ts))
	for _, t := range ts {
		m[tripleKey(t)] = struct{}{}
	}
	return m
}
