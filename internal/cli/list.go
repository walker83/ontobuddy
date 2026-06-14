package cli

import (
	"encoding/json"
	"io"
	"os"
)

// ListCmd 是 entity list 的快捷别名，直接复用 EntityListCmd。
type ListCmd struct {
	EntityListCmd
}

// --- JSON 输出辅助 ---

// EntitySummary 是 list / search 命令的 JSON 输出格式。
//
// 设计原则：稳定、轻量、自描述。LLM 解析这个就能列出本体全貌。
type EntitySummary struct {
	Local     string   `json:"local"`
	IRI       string   `json:"iri"`
	Label     string   `json:"label"`
	Types     []string `json:"types"`
	Desc      string   `json:"desc,omitempty"`
	MatchKind string   `json:"match_kind,omitempty"`
}

// printEntitySummariesJSON 把实体摘要列表以 JSON 格式输出到 stdout。
func printEntitySummariesJSON(summaries []EntitySummary) {
	enc := jsonOut(os.Stdout)
	_ = enc.Encode(map[string]any{
		"entities": summaries,
		"count":    len(summaries),
	})
}

// jsonOut 构造一个缩进 2 空的 JSON encoder（统一 CLI 各命令的 JSON 风格）。
func jsonOut(w io.Writer) *json.Encoder {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
