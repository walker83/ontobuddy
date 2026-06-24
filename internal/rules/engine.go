package rules

import (
	"sort"

	"github.com/walker/myonto/internal/rdf"
)

// RuleStats 记录单条规则的推理统计。
type RuleStats struct {
	RuleID         string `json:"rule_id"`
	RuleName       string `json:"rule_name"`
	ProducedTriples int   `json:"produced_triples"`
	Iterations      int   `json:"iterations"`
}

// EngineResult 是推理引擎的执行结果。
type EngineResult struct {
	Derived []rdf.Triple          `json:"derived"`
	Stats   map[string]RuleStats  `json:"stats"`
}

// Engine 是基于外部规则定义的推理引擎。
type Engine struct {
	rules   []RuleDef
	ctx     *Context
}

// NewEngine 创建推理引擎。
// rules 是加载好的规则定义列表；ctx 提供内置规则需要的上下文。
func NewEngine(rules []RuleDef, ctx *Context) *Engine {
	return &Engine{
		rules: rules,
		ctx:   ctx,
	}
}

// UpdateRules 更新引擎的规则定义（用于运行时启用/禁用规则）。
func (e *Engine) UpdateRules(rules []RuleDef) {
	e.rules = rules
}

// Rules 返回当前规则定义列表。
func (e *Engine) Rules() []RuleDef {
	return e.rules
}

// Derive 对 base 三元组执行所有启用的规则直到不动点，
// 返回新推导的三元组和每条规则的统计信息。
func (e *Engine) Derive(base []rdf.Triple) EngineResult {
	known := make([]rdf.Triple, len(base))
	copy(known, base)
	knownSet := tripleKeySet(known)

	stats := make(map[string]RuleStats)

	// 收集启用的规则
	type activeRule struct {
		def  RuleDef
		fn   BuiltinFunc
	}
	var active []activeRule
	for _, def := range e.rules {
		if !def.Enabled {
			continue
		}
		fn, ok := GetBuiltinFunc(def.ID)
		if !ok {
			// 没有对应的内置实现，跳过
			continue
		}
		active = append(active, activeRule{def: def, fn: fn})
	}

	// 不动点循环
	for {
		var newOnes []rdf.Triple
		for _, ar := range active {
			before := len(knownSet)
			got := ar.fn(known, e.ctx)
			for _, t := range got {
				k := tripleKeyStr(t)
				if _, exists := knownSet[k]; exists {
					continue
				}
				knownSet[k] = struct{}{}
				known = append(known, t)
				newOnes = append(newOnes, t)
			}
			produced := len(knownSet) - before
			if produced > 0 {
				s := stats[ar.def.ID]
				s.RuleID = ar.def.ID
				s.RuleName = ar.def.Name
				s.ProducedTriples += produced
				s.Iterations++
				stats[ar.def.ID] = s
			}
		}
		if len(newOnes) == 0 {
			break
		}
	}

	// 过滤掉原始三元组
	originalSet := tripleKeySet(base)
	out := make([]rdf.Triple, 0, len(known)-len(base))
	for _, t := range known {
		if _, ok := originalSet[tripleKeyStr(t)]; ok {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })

	return EngineResult{Derived: out, Stats: stats}
}

// --- 工具函数 ---

func tripleKeyStr(t rdf.Triple) string { return t.String() }

func tripleKeySet(ts []rdf.Triple) map[string]struct{} {
	m := make(map[string]struct{}, len(ts))
	for _, t := range ts {
		m[tripleKeyStr(t)] = struct{}{}
	}
	return m
}
