// Package model 提供领域模型辅助：把人类可读的名字转为本体中的标识。
package model

import (
	"strings"
	"unicode"
)

// Slug 把任意名字转换成适合作为 IRI local name 的 slug。
// 规则：
//   - 转小写
//   - 空白与连字符序列替换为单个连字符
//   - 去除所有非 [a-z0-9-] 字符（中文等非 ASCII 也去除，保证 local name 合法）
//   - 首尾连字符裁掉
//
// 注意：本项目约定 local name 仅用 ASCII，对中文等输入会丢失信息，
// 建议在 add 时同时提供 rdfs:label 保留原始名。
func Slug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	prevDash := true // 开头不输出连字符
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case unicode.IsSpace(r) || r == '_' || r == '-':
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		default:
			// 非 ASCII（含中文）及其他符号：用连字符占位，避免丢词边界。
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		// 全部被过滤（例如纯中文），fallback 用 "entity"。
		return "entity"
	}
	return out
}
