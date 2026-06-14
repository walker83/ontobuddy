package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/walker/myonto/internal/model"
)

// ImportCmd 从 markdown / 纯文本文件读，输出结构化 JSON 草稿。
//
// 重要：这个命令**不调用 LLM**。它只做"文本 → 结构化骨架"的确定性转换：
//   - 提取标题（# 一级）作为候选实体名
//   - 提取列表项（- xxx）作为候选属性
//   - 提取表格行作为结构化数据
//   - 输出与 `entity apply` 兼容的 JSON 数组
//
// 外部 LLM（Claude/opencode）读输出后加工（消歧、归类、补关系），
// 再用 `entity apply` 写入本体。这样 LLM 在你这边跑，myonto 只做苦力。
type ImportCmd struct {
	Path string `arg:"" required:"" placeholder:"PATH" help:"md/txt 文件或目录"`
	Out  string `short:"o" help:"输出 JSON 路径；默认 stdout" placeholder:"FILE"`
}

// importItem 是 import 输出的格式（与 entity apply 输入兼容）。
type importItem = entityApplyItem

// Run 执行 import。
func (c *ImportCmd) Run() error {
	files, err := expandImportPath(c.Path)
	if err != nil {
		return err
	}
	var allItems []importItem
	for _, f := range files {
		items := parseMarkdownFile(f)
		allItems = append(allItems, items...)
	}

	data, err := json.MarshalIndent(map[string]any{
		"source_files": files,
		"items":        allItems,
		"item_count":   len(allItems),
	}, "", "  ")
	if err != nil {
		return err
	}

	if c.Out != "" {
		return os.WriteFile(c.Out, data, 0o644)
	}
	fmt.Fprintln(os.Stdout, string(data))
	return nil
}

// expandImportPath 把 path 展开成文件列表（目录则递归找 .md/.txt）。
func expandImportPath(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{path}, nil
	}
	var files []string
	err = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext == ".md" || ext == ".txt" {
			files = append(files, p)
		}
		return nil
	})
	return files, err
}

// parseMarkdownFile 简单解析 md 文件，提取候选实体和属性。
//
// 启发式（不调 LLM）：
//   - "# 标题" → 主实体名
//   - "## 子标题" → 候选属性 key
//   - "- xxx: yyy" → 属性值
//   - "| a | b |" → 表格行（暂存为 desc）
func parseMarkdownFile(path string) []importItem {
	data, err := io.ReadAll(func() io.ReadCloser {
		f, e := os.Open(path)
		if e != nil {
			return nil
		}
		return f
	}())
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")

	baseName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	mainItem := importItem{
		Name:  baseName,
		Label: baseName,
	}
	var currentSection string
	var descParts []string

	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "# ") {
			// 一级标题覆盖主实体 label
			mainItem.Label = strings.TrimPrefix(trim, "# ")
			continue
		}
		if strings.HasPrefix(trim, "## ") {
			currentSection = strings.TrimPrefix(trim, "## ")
			continue
		}
		// 列表项 "- key: value" 或 "- value"
		if strings.HasPrefix(trim, "- ") {
			body := strings.TrimPrefix(trim, "- ")
			if i := strings.Index(body, ":"); i > 0 {
				key := strings.TrimSpace(body[:i])
				val := strings.TrimSpace(body[i+1:])
				descParts = append(descParts, fmt.Sprintf("%s: %s", key, val))
			} else if body != "" {
				descParts = append(descParts, body)
			}
			continue
		}
		// 普通段落（非空非表格分隔）作为 desc 补充
		if trim != "" && !strings.HasPrefix(trim, "|") && !strings.HasPrefix(trim, "---") {
			if currentSection != "" {
				descParts = append(descParts, fmt.Sprintf("[%s] %s", currentSection, trim))
			}
		}
	}
	if len(descParts) > 0 {
		mainItem.Desc = strings.Join(descParts, "\n")
	}
	// 主实体的 type 留空——让外部 LLM 根据现有 schema 决定归到哪个类
	_ = model.Slug // 确保 import 不被裁掉
	return []importItem{mainItem}
}
