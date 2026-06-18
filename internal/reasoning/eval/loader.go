package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/walker/myonto/internal/rdf"
)

// Case 是单个评估用例的内存表示。
type Case struct {
	Name            string       // 用例名（取自 case.json，回退到目录名）
	Description     string       // 人类可读描述
	Rule            string       // 对应的规则名（如 "subClassOf-transitive"）
	Input           []rdf.Triple // 输入本体
	ExpectDerive    []rdf.Triple // 期望推出（正例，漏 = FN）
	ExpectNotDerive []rdf.Triple // 绝不应推出（负例，推 = FP）
}

// caseMeta 是 case.json 的结构。
type caseMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Rule        string `json:"rule"`
}

// LoadCase 从一个 case 目录加载用例。
//
// 目录结构：
//
//	<dir>/
//	  case.json              # 元数据（name/description/rule），可选
//	  input.ttl              # 输入本体（必需）
//	  expect_derive.ttl      # 期望推出的三元组（可空/缺省）
//	  expect_not_derive.ttl  # 绝不应推出的三元组（可空/缺省）
//
// case.json 缺省时 Name 取目录名。
func LoadCase(dir string) (Case, error) {
	dir = filepath.Clean(dir)
	var c Case

	// 元数据
	if metaBytes, err := os.ReadFile(filepath.Join(dir, "case.json")); err == nil {
		var meta caseMeta
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			return c, fmt.Errorf("case %s: 解析 case.json: %w", dir, err)
		}
		c.Name, c.Description, c.Rule = meta.Name, meta.Description, meta.Rule
	}
	if c.Name == "" {
		c.Name = filepath.Base(dir)
	}

	// 输入本体（必需）
	inputTTL, err := os.ReadFile(filepath.Join(dir, "input.ttl"))
	if err != nil {
		return c, fmt.Errorf("case %s: 读 input.ttl: %w", c.Name, err)
	}
	triples, _, err := rdf.ParseTurtle(string(inputTTL))
	if err != nil {
		return c, fmt.Errorf("case %s: 解析 input.ttl: %w", c.Name, err)
	}
	c.Input = triples

	// 正例（可选）
	if t, err := loadTTLFile(dir, "expect_derive.ttl"); err != nil {
		return c, fmt.Errorf("case %s: %w", c.Name, err)
	} else {
		c.ExpectDerive = t
	}

	// 负例（可选）
	if t, err := loadTTLFile(dir, "expect_not_derive.ttl"); err != nil {
		return c, fmt.Errorf("case %s: %w", c.Name, err)
	} else {
		c.ExpectNotDerive = t
	}

	return c, nil
}

// loadTTLFile 读一个 ttl 文件并解析为三元组；文件不存在时返回空切片（非错误）。
func loadTTLFile(dir, name string) ([]rdf.Triple, error) {
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读 %s: %w", name, err)
	}
	triples, _, err := rdf.ParseTurtle(string(data))
	if err != nil {
		return nil, fmt.Errorf("解析 %s: %w", name, err)
	}
	return triples, nil
}

// LoadCases 从一个父目录加载所有子目录中的 case，按 name 排序。
// 只收集含 input.ttl 的子目录，跳过其他文件。
func LoadCases(parentDir string) ([]Case, error) {
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return nil, fmt.Errorf("读 case 目录 %s: %w", parentDir, err)
	}
	var cases []Case
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// 只认含 input.ttl 的目录，跳过 README 等非 case 目录。
		if _, err := os.Stat(filepath.Join(parentDir, e.Name(), "input.ttl")); err != nil {
			continue
		}
		c, err := LoadCase(filepath.Join(parentDir, e.Name()))
		if err != nil {
			return nil, err
		}
		cases = append(cases, c)
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].Name < cases[j].Name })
	return cases, nil
}
