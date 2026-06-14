package model

import "testing"

func TestSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Isaac Newton", "isaac-newton"},
		{"  trim  ", "trim"},
		{"Multiple   Spaces", "multiple-spaces"},
		{"Already-slug", "already-slug"},
		{"UPPER_CASE", "upper-case"},
		{"中文纯文本", "entity"}, // 非 ASCII 全过滤后 fallback
		{"Mixed 中文 Test", "mixed-test"},
		{"with!@#punct", "with-punct"},
		{"---leading", "leading"},
		{"trailing---", "trailing"},
		{"", "entity"}, // 空字符串 fallback
		{"123", "123"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := Slug(tt.input); got != tt.want {
				t.Errorf("Slug(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
