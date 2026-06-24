// Package rules 实现推理规则的外部化定义与管理。
//
// 规则通过 YAML 文件定义，支持：
//   - 链式（chain）规则：声明式模式匹配，变量绑定 + 结论模板
//   - 内置（builtin）规则：复杂逻辑（如 BFS 闭包）由 Go 代码实现
//
// 规则加载优先级：
//  1. .myonto/rules.yaml（项目本地覆盖）
//  2. ~/.config/myonto/rules.yaml（用户全局覆盖）
//  3. 内嵌 default.yaml（始终可用）
package rules

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed default.yaml
var defaultYAML []byte

// RuleDef 是一条推理规则的完整定义。
type RuleDef struct {
	ID             string    `yaml:"id" json:"id"`
	Name           string    `yaml:"name" json:"name"`
	Description    string    `yaml:"description" json:"description"`
	Category       string    `yaml:"category" json:"category"` // rdfs, owl, custom
	Enabled        bool      `yaml:"enabled" json:"enabled"`
	Implementation string    `yaml:"implementation" json:"implementation"` // "builtin" or "declarative"
	Spec           RuleSpec  `yaml:"spec" json:"spec"`
}

// RuleSpec 描述规则的推理模式。
type RuleSpec struct {
	Type        string         `yaml:"type" json:"type"`                 // "chain" or "builtin"
	BuiltinID   string         `yaml:"builtin_id,omitempty" json:"builtin_id,omitempty"`
	BuiltinDesc string         `yaml:"description,omitempty" json:"description,omitempty"`
	Premises    []Pattern      `yaml:"premises,omitempty" json:"premises,omitempty"`
	Conclusion  *Pattern       `yaml:"conclusion,omitempty" json:"conclusion,omitempty"`
	Filters     []string       `yaml:"filters,omitempty" json:"filters,omitempty"`
}

// Pattern 是一个三元组模式，包含主语、谓词、宾语。
// 值可以是变量（?x）或固定 IRI（如 "rdf:type"）。
type Pattern struct {
	S string `yaml:"s" json:"s"`
	P string `yaml:"p" json:"p"`
	O string `yaml:"o" json:"o"`
}

// RulesFile 是 YAML 规则文件的顶层结构。
type RulesFile struct {
	Rules []RuleDef `yaml:"rules"`
}

// LoadDefault 加载内嵌的默认规则。
func LoadDefault() ([]RuleDef, error) {
	return ParseRules(defaultYAML)
}

// ParseRules 从 YAML 字节解析规则定义。
func ParseRules(data []byte) ([]RuleDef, error) {
	var f RulesFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse rules yaml: %w", err)
	}
	// 验证每条规则
	for i, r := range f.Rules {
		if r.ID == "" {
			return nil, fmt.Errorf("rule[%d]: missing id", i)
		}
		if r.Name == "" {
			return nil, fmt.Errorf("rule[%d] (%s): missing name", i, r.ID)
		}
		if r.Spec.Type == "" {
			return nil, fmt.Errorf("rule[%d] (%s): missing spec.type", i, r.ID)
		}
		if r.Implementation == "" {
			f.Rules[i].Implementation = "builtin"
		}
	}
	return f.Rules, nil
}

// LoadFile 从文件路径加载规则。
func LoadFile(path string) ([]RuleDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read rules file %s: %w", path, err)
	}
	return ParseRules(data)
}

// LoadWithOverrides 按优先级加载规则：项目本地 > 用户全局 > 内嵌默认。
func LoadWithOverrides(projectDir string) ([]RuleDef, error) {
	base, err := LoadDefault()
	if err != nil {
		return nil, err
	}

	// 用户全局覆盖
	home, _ := os.UserHomeDir()
	if home != "" {
		globalPath := filepath.Join(home, ".config", "myonto", "rules.yaml")
		if data, err := os.ReadFile(globalPath); err == nil {
			override, err := ParseRules(data)
			if err == nil {
				base = mergeRules(base, override)
			}
		}
	}

	// 项目本地覆盖
	if projectDir != "" {
		localPath := filepath.Join(projectDir, ".myonto", "rules.yaml")
		if data, err := os.ReadFile(localPath); err == nil {
			override, err := ParseRules(data)
			if err == nil {
				base = mergeRules(base, override)
			}
		}
	}

	return base, nil
}

// SaveRules 将规则定义保存到 YAML 文件。
func SaveRules(path string, rules []RuleDef) error {
	f := RulesFile{Rules: rules}
	data, err := yaml.Marshal(&f)
	if err != nil {
		return fmt.Errorf("marshal rules: %w", err)
	}
	header := "# MyOntopo 推理规则配置\n# 修改此文件可自定义推理行为\n\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(header+string(data)), 0o644)
}

// mergeRules 合并 override 到 base：按 ID 匹配，override 中存在的规则替换 base 中的同 ID 规则，
// override 中的新规则追加到末尾。
func mergeRules(base, override []RuleDef) []RuleDef {
	idx := make(map[string]int, len(base))
	for i, r := range base {
		idx[r.ID] = i
	}
	for _, r := range override {
		if i, ok := idx[r.ID]; ok {
			base[i] = r // 替换
		} else {
			base = append(base, r) // 追加
		}
	}
	return base
}

// RuleInfo 是面向 API/前端的规则信息摘要。
type RuleInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Category       string `json:"category"`
	Enabled        bool   `json:"enabled"`
	Implementation string `json:"implementation"`
	SpecType       string `json:"spec_type"`
	PremiseCount   int    `json:"premise_count,omitempty"`
	HasConclusion  bool   `json:"has_conclusion,omitempty"`
	FilterCount    int    `json:"filter_count,omitempty"`
	// Stats 在推理后填充
	ProducedTriples int `json:"produced_triples,omitempty"`
	Iterations      int `json:"iterations,omitempty"`
}

// ToInfo 将 RuleDef 转换为 API 响应格式。
func (r RuleDef) ToInfo() RuleInfo {
	info := RuleInfo{
		ID:             r.ID,
		Name:           r.Name,
		Description:    r.Description,
		Category:       r.Category,
		Enabled:        r.Enabled,
		Implementation: r.Implementation,
		SpecType:       r.Spec.Type,
	}
	if r.Spec.Conclusion != nil {
		info.HasConclusion = true
	}
	info.PremiseCount = len(r.Spec.Premises)
	info.FilterCount = len(r.Spec.Filters)
	return info
}
