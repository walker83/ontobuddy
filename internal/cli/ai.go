package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/walker/myonto/internal/llm"
	"github.com/walker/myonto/internal/rdf"
	"github.com/walker/myonto/internal/store"
)

// AICmd 是 myonto ai 的命令组：用 LLM 辅助整理本体。
//
// 所有 ai 命令默认 dry-run：把 LLM 输出打印到 stdout，等用户审视后加 --apply 才写回本体。
// 这是为了防止 LLM 幻觉污染数据。
//
// 支持 --json：把 LLM 的 prompt 构造 + 响应一并以结构化形式输出，
// 供其他程序（如 Skill 包装）调用。
type AICmd struct {
	Summarize        AISummarizeCmd        `cmd:"" help:"让 LLM 归纳某实体的所有三元组"`
	Extract          AIExtractCmd          `cmd:"" help:"从自然语言文本抽取实体与关系（生成 Turtle 草稿）"`
	SuggestRelations AISuggestRelationsCmd `cmd:"" help:"基于上下文让 LLM 建议可能的关系"`
	QA               AIQACmd               `cmd:"" help:"基于本体的问答"`
}

// --- AI summarize ---

// AISummarizeCmd 让 LLM 归纳某实体的所有三元组。
type AISummarizeCmd struct {
	Entity string `arg:"" required:"" placeholder:"ENTITY" help:"实体名（local name 或 IRI）"`
	Apply  bool   `short:"a" help:"把生成的 summary 写入实体的 rdfs:comment（默认仅打印）"`
}

func (c *AISummarizeCmd) Run() error {
	s, cfgPath, err := openStore()
	if err != nil {
		return err
	}
	subj, err := s.ResolveName(c.Entity)
	if err != nil {
		return err
	}
	triples := s.Query(rdf.Triple{Subject: subj})
	if len(triples) == 0 {
		return fmt.Errorf("实体不存在：%s", c.Entity)
	}

	prompt := buildSummaryPrompt(subj, triples, s)
	out, err := callLLM(s, []llm.Message{
		{Role: "system", Content: "你是一个本体编辑助手，擅长用中文简洁地总结 RDF 数据。"},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return err
	}

	if IsJSON() {
		enc := jsonOut(os.Stdout)
		return enc.Encode(aiSummaryResult{
			Entity:    c.Entity,
			EntityIRI: subj.Value,
			Triples:   len(triples),
			Prompt:    prompt,
			LLMOutput: out,
			WillApply: c.Apply,
		})
	}

	fmt.Fprintf(os.Stdout, "=== %s 的总结 ===\n%s\n", c.Entity, out)
	if c.Apply {
		s.Remove(rdf.Triple{Subject: subj, Predicate: rdf.Comment})
		s.Add(rdf.Triple{Subject: subj, Predicate: rdf.Comment, Object: rdf.Lit(strings.TrimSpace(out))})
		if err := saveStore(s, cfgPath); err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, "✓ 已写入 rdfs:comment")
	} else {
		fmt.Fprintln(os.Stdout, "（加 -a 写回 rdfs:comment）")
	}
	return nil
}

// buildSummaryPrompt 构造 summarize 的 prompt。
func buildSummaryPrompt(subj rdf.Term, triples []rdf.Triple, _ *store.Store) string {
	var b strings.Builder
	fmt.Fprintf(&b, "请用 1-3 句中文总结以下实体：\n\n")
	fmt.Fprintf(&b, "实体：%s\n", subj.LocalName())
	fmt.Fprintf(&b, "它的全部三元组：\n")
	for _, t := range triples {
		fmt.Fprintf(&b, "  %s %s %s\n", shortTerm(t.Subject), shortTerm(t.Predicate), shortTerm(t.Object))
	}
	b.WriteString("\n要求：简洁、信息密度高、不超过 100 字。")
	return b.String()
}

// --- AI extract ---

// AIExtractCmd 从自然语言文本抽取实体与关系。
type AIExtractCmd struct {
	Text  string `arg:"" required:"" placeholder:"TEXT" help:"自然语言文本（中文或英文）"`
	Apply bool   `short:"a" help:"把生成的 Turtle 合并到本体（默认仅展示）"`
}

func (c *AIExtractCmd) Run() error {
	s, cfgPath, err := openStore()
	if err != nil {
		return err
	}
	prompt := buildExtractPrompt(c.Text, s)
	out, err := callLLM(s, []llm.Message{
		{Role: "system", Content: "你是一个本体构建助手，擅长从自然语言文本中抽取实体与关系，输出标准 Turtle 格式。"},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return err
	}
	turtle := extractTurtleBlock(out)

	if IsJSON() {
		enc := jsonOut(os.Stdout)
		return enc.Encode(aiExtractResult{
			InputText: c.Text,
			Prompt:    prompt,
			LLMOutput: out,
			Turtle:    turtle,
			WillApply: c.Apply,
		})
	}

	fmt.Fprintln(os.Stdout, "=== LLM 生成的 Turtle 草稿 ===")
	fmt.Fprintln(os.Stdout, turtle)
	if c.Apply {
		if err := mergeTurtleInto(s, turtle); err != nil {
			return err
		}
		if err := saveStore(s, cfgPath); err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, "✓ 已合并到本体")
	} else {
		fmt.Fprintln(os.Stdout, "（加 -a 合并到本体；请人工审阅 LLM 输出，避免幻觉污染）")
	}
	return nil
}

// buildExtractPrompt 构造 extract 的 prompt。
func buildExtractPrompt(text string, s *store.Store) string {
	prefixes := s.Prefixes()
	var prefixList strings.Builder
	for k, v := range prefixes {
		fmt.Fprintf(&prefixList, "@prefix %s: <%s> .\n", k, v)
	}
	return fmt.Sprintf(`从以下文本中抽取实体、类、关系，输出**纯 Turtle 格式**（不要 Markdown 包裹，不要解释文字）。

可用的前缀：
%s

文本：
"""%s"""

要求：
1. 抽取所有出现的人/物/概念作为实体
2. 抽取出明确的类型关系（rdfs:label "中文名" ; rdf:type ex:SomeClass）
3. 抽取出明确的关系（自定义谓词用 ex:verb 形式）
4. 输出必须以 @prefix 开始（如果用了任何前缀），且每条三元组以 . 结尾
5. 不确定的就不要猜`, prefixList.String(), text)
}

// extractTurtleBlock 从 LLM 响应中提取 Turtle 代码块（去 ```turtle 包裹）。
func extractTurtleBlock(out string) string {
	out = strings.TrimSpace(out)
	if i := strings.Index(out, "```"); i >= 0 {
		rest := out[i+3:]
		if nl := strings.Index(rest, "\n"); nl >= 0 && !strings.Contains(rest[:nl], "```") {
			firstLine := strings.TrimSpace(rest[:nl])
			if firstLine == "turtle" || firstLine == "ttl" {
				rest = rest[nl+1:]
			}
		}
		if j := strings.LastIndex(rest, "```"); j >= 0 {
			out = rest[:j]
		}
	}
	return strings.TrimSpace(out)
}

// mergeTurtleInto 解析一段 Turtle 文本，把三元组合并到 store。
func mergeTurtleInto(s *store.Store, turtle string) error {
	triples, prefixes, err := rdf.ParseTurtle(turtle)
	if err != nil {
		return fmt.Errorf("解析 LLM 输出的 Turtle 失败：%w", err)
	}
	s.AddPrefixes(prefixes)
	for _, t := range triples {
		s.Add(t)
	}
	return nil
}

// --- AI suggest-relations ---

// AISuggestRelationsCmd 基于上下文让 LLM 建议可能的关系。
type AISuggestRelationsCmd struct {
	Entity string `arg:"" required:"" placeholder:"ENTITY"`
	Apply  bool   `short:"a" help:"把建议的关系直接写入本体（默认仅展示）"`
}

func (c *AISuggestRelationsCmd) Run() error {
	s, cfgPath, err := openStore()
	if err != nil {
		return err
	}
	subj, err := s.ResolveName(c.Entity)
	if err != nil {
		return err
	}
	triples := s.Query(rdf.Triple{Subject: subj})
	if len(triples) == 0 {
		return fmt.Errorf("实体不存在：%s", c.Entity)
	}

	prompt := buildSuggestPrompt(subj, triples, s)
	out, err := callLLM(s, []llm.Message{
		{Role: "system", Content: "你是一个本体补全助手，擅长基于现有 RDF 数据建议可能有用的新三元组。"},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return err
	}
	turtle := extractTurtleBlock(out)

	if IsJSON() {
		enc := jsonOut(os.Stdout)
		return enc.Encode(aiSuggestResult{
			Entity:    c.Entity,
			Prompt:    prompt,
			LLMOutput: out,
			Turtle:    turtle,
			WillApply: c.Apply,
		})
	}

	fmt.Fprintln(os.Stdout, "=== LLM 建议的关系（Turtle 草稿）===")
	fmt.Fprintln(os.Stdout, turtle)
	if c.Apply {
		if err := mergeTurtleInto(s, turtle); err != nil {
			return err
		}
		if err := saveStore(s, cfgPath); err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, "✓ 已合并到本体")
	} else {
		fmt.Fprintln(os.Stdout, "（加 -a 合并到本体；请人工审阅 LLM 输出）")
	}
	return nil
}

// buildSuggestPrompt 构造 suggest-relations 的 prompt。
func buildSuggestPrompt(subj rdf.Term, triples []rdf.Triple, s *store.Store) string {
	var b strings.Builder
	fmt.Fprintf(&b, "现有实体 %s 的三元组：\n", subj.LocalName())
	for _, t := range triples {
		fmt.Fprintf(&b, "  %s %s %s\n", shortTerm(t.Subject), shortTerm(t.Predicate), shortTerm(t.Object))
	}
	b.WriteString("\n本体的其他相关实体（用 ex:localname 引用）：\n")
	subjects := s.Subjects()
	count := 0
	for _, x := range subjects {
		if x.Equal(subj) {
			continue
		}
		fmt.Fprintf(&b, "  ex:%s\n", x.LocalName())
		count++
		if count >= 50 {
			b.WriteString("  ...(更多省略)\n")
			break
		}
	}
	b.WriteString(`
请建议 3-8 条**新增的**三元组，让这个实体的知识更完整。
要求：
- 只输出 Turtle 三元组，每条以 . 结尾
- 不要重复列出已有三元组
- 用 ex:xxx 形式引用已存在的实体
- 不确定的不要猜

输出示例：
ex:newton ex:discovered ex:gravity .
ex:newton ex:workedAt ex:royal-society .
`)
	return b.String()
}

// --- AI qa ---

// AIQACmd 基于本体的问答。
type AIQACmd struct {
	Question string `arg:"" required:"" placeholder:"QUESTION" help:"用自然语言提问"`
}

func (c *AIQACmd) Run() error {
	s, _, err := openStore()
	if err != nil {
		return err
	}

	triples := s.Triples()
	if len(triples) == 0 {
		return fmt.Errorf("本体为空，无法问答")
	}
	var ctx strings.Builder
	for _, t := range triples {
		fmt.Fprintf(&ctx, "%s %s %s .\n", shortTerm(t.Subject), shortTerm(t.Predicate), shortTerm(t.Object))
	}

	prompt := fmt.Sprintf("以下是一个 RDF 本体的全部三元组：\n\n%s\n\n问题：%s\n\n请基于上述本体的信息回答。如果本体中没有相关信息，请明确说「本体中无相关信息」。", ctx.String(), c.Question)
	out, err := callLLM(s, []llm.Message{
		{Role: "system", Content: "你是一个基于 RDF 本体回答问题的助手，只根据给定的三元组信息回答，不编造。"},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return err
	}

	if IsJSON() {
		enc := jsonOut(os.Stdout)
		return enc.Encode(aiQAResult{
			Question: c.Question,
			Prompt:   prompt,
			Answer:   out,
		})
	}

	fmt.Fprintln(os.Stdout, out)
	return nil
}

// --- AI JSON 输出结构 ---

type aiSummaryResult struct {
	Entity    string `json:"entity"`     // 用户输入的实体名
	EntityIRI string `json:"entity_iri"` // 解析后的 IRI
	Triples   int    `json:"triples"`    // 喂给 LLM 的三元组数
	Prompt    string `json:"prompt"`     // 构造的 prompt（便于复现 / 调试）
	LLMOutput string `json:"llm_output"` // LLM 原始输出
	WillApply bool   `json:"will_apply"` // 是否会物化
}

type aiExtractResult struct {
	InputText string `json:"input_text"` // 用户输入的自然语言
	Prompt    string `json:"prompt"`
	LLMOutput string `json:"llm_output"`
	Turtle    string `json:"turtle"` // 解析后的 Turtle 草稿
	WillApply bool   `json:"will_apply"`
}

type aiSuggestResult struct {
	Entity    string `json:"entity"`
	Prompt    string `json:"prompt"`
	LLMOutput string `json:"llm_output"`
	Turtle    string `json:"turtle"`
	WillApply bool   `json:"will_apply"`
}

type aiQAResult struct {
	Question string `json:"question"`
	Prompt   string `json:"prompt"`
	Answer   string `json:"answer"`
}

// --- 共用 ---

// callLLM 构造客户端并调用 chat，超时 60s。
func callLLM(s *store.Store, msgs []llm.Message) (string, error) {
	c := llm.FromConfig(s.Config().LLM)
	if !c.Available() {
		return "", fmt.Errorf("LLM 未配置：在 .myonto.toml 的 [llm] 节设置 base_url/api_key/model，或用环境变量 MYONTO_LLM_BASE_URL / MYONTO_LLM_API_KEY / MYONTO_LLM_MODEL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return c.Chat(ctx, msgs)
}
