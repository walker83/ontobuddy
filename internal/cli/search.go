package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/walker/myonto/internal/rdf"
)

// SearchCmd 在 label / comment / local name 上做子串匹配。
// 支持 --json（受全局 --json 影响）。
type SearchCmd struct {
	Keyword string `arg:"" required:"" placeholder:"KEYWORD" help:"搜索关键词（在 local name / rdfs:label / rdfs:comment 上做子串匹配）"`
	Type    string `short:"t" help:"只搜该类型的实例" placeholder:"CLASS"`
}

func (c *SearchCmd) Run() error {
	s, _, err := openStore()
	if err != nil {
		return err
	}
	kw := strings.ToLower(c.Keyword)
	var typeFilter rdf.Term
	if c.Type != "" {
		typeFilter, err = s.ResolveName(c.Type)
		if err != nil {
			return err
		}
	}

	hits := 0
	var summaries []EntitySummary
	for _, subj := range s.Subjects() {
		// 类型过滤
		if c.Type != "" {
			if len(s.Query(rdf.Triple{Subject: subj, Predicate: rdf.Type, Object: typeFilter})) == 0 {
				continue
			}
		}
		matched := false
		reason := ""
		if strings.Contains(strings.ToLower(subj.LocalName()), kw) {
			matched = true
			reason = "name"
		}
		if !matched {
			for _, t := range s.Query(rdf.Triple{Subject: subj, Predicate: rdf.Label}) {
				if strings.Contains(strings.ToLower(t.Object.Value), kw) {
					matched = true
					reason = "label"
					break
				}
			}
		}
		if !matched {
			for _, t := range s.Query(rdf.Triple{Subject: subj, Predicate: rdf.Comment}) {
				if strings.Contains(strings.ToLower(t.Object.Value), kw) {
					matched = true
					reason = "desc"
					break
				}
			}
		}
		if !matched {
			continue
		}

		if IsJSON() {
			sum := buildSummary(s, subj)
			sum.MatchKind = reason
			summaries = append(summaries, sum)
		} else {
			desc := ""
			for _, t := range s.Query(rdf.Triple{Subject: subj, Predicate: rdf.Comment}) {
				desc = t.Object.Value
				break
			}
			if len(desc) > 50 {
				desc = desc[:50] + "…"
			}
			fmt.Fprintf(os.Stdout, "  %-20s  (%s) %s\n", subj.LocalName(), reason, desc)
		}
		hits++
	}
	if IsJSON() {
		printEntitySummariesJSON(summaries)
		return nil
	}
	if hits == 0 {
		fmt.Fprintln(os.Stdout, "（无匹配）")
	}
	return nil
}
