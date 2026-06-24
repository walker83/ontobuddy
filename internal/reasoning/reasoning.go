// Package reasoning 实现轻量的 RDFS/OWL 规则推理。
//
// 规则通过 internal/rules 包外部化定义，支持：
//   - 链式（chain）规则：声明式模式匹配
//   - 内置（builtin）规则：复杂逻辑由 Go 代码实现
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
package reasoning

import (
	"github.com/walker/myonto/internal/rdf"
	"github.com/walker/myonto/internal/rules"
)

// Reasoner 是对一组三元组跑规则推理的引擎。
// 构造时传入 base 三元组；Derive() 在 base 之上跑规则，输出新推导的三元组。
type Reasoner struct {
	base   []rdf.Triple   // 构造时的原始三元组（不动）
	engine *rules.Engine  // 推理引擎
	ctx    *rules.Context  // 推理上下文（谓词缓存）
}

// Option 是 Reasoner 的可选配置。
type Option func(*Reasoner)

// WithRules 使用自定义规则列表（而非默认规则）。
func WithRules(r []rules.RuleDef) Option {
	return func(reasoner *Reasoner) {
		reasoner.engine = rules.NewEngine(r, reasoner.ctx)
	}
}

// WithEngine 使用自定义推理引擎。
func WithEngine(e *rules.Engine) Option {
	return func(reasoner *Reasoner) {
		reasoner.engine = e
	}
}

// NewReasoner 从 base 三元组中预扫描谓词类型信息，并创建推理引擎。
func NewReasoner(base []rdf.Triple, opts ...Option) *Reasoner {
	// 复制一份避免外部修改影响内部。
	cp := make([]rdf.Triple, len(base))
	copy(cp, base)

	// 预扫描谓词类型信息
	ctx := &rules.Context{
		Transitive: map[rdf.Term]bool{},
		Symmetric:  map[rdf.Term]bool{},
		Inverses:   map[rdf.Term]rdf.Term{},
	}
	for _, t := range cp {
		if t.Predicate.Equal(rdf.Type) {
			if t.Object.Equal(rdf.TransitiveProperty) {
				ctx.Transitive[t.Subject] = true
			}
			if t.Object.Equal(rdf.SymmetricProperty) {
				ctx.Symmetric[t.Subject] = true
			}
		}
		if t.Predicate.Equal(rdf.InverseOf) {
			ctx.Inverses[t.Subject] = t.Object
		}
	}

	r := &Reasoner{
		base: cp,
		ctx:  ctx,
	}

	// 加载默认规则并创建引擎
	defs, _ := rules.LoadDefault()
	r.engine = rules.NewEngine(defs, ctx)

	// 应用可选配置
	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Derive 在构造时传入的 base 之上跑全部规则直到不动点，
// 返回所有推导出的新三元组（不含原始三元组；与原始去重）。
func (r *Reasoner) Derive() []rdf.Triple {
	result := r.engine.Derive(r.base)
	return result.Derived
}

// DeriveWithStats 在构造时传入的 base 之上跑全部规则直到不动点，
// 返回推导出的新三元组和每条规则的统计信息。
func (r *Reasoner) DeriveWithStats() ([]rdf.Triple, map[string]rules.RuleStats) {
	result := r.engine.Derive(r.base)
	return result.Derived, result.Stats
}

// Engine 返回底层的推理引擎（用于 API 层操作规则）。
func (r *Reasoner) Engine() *rules.Engine {
	return r.engine
}

// Base 返回原始三元组（只读）。
func (r *Reasoner) Base() []rdf.Triple {
	return r.base
}
