package rdf

import (
	"fmt"
	"strings"
	"unicode"
)

// ParseTurtle 解析 Turtle 文本，返回三元组列表与解析到的前缀映射。
// 支持本项目所需的子集：
//   - @prefix 声明（含空前缀 @prefix : <...>）
//   - PREFIX 关键字（SPARQL 风格，兼容）
//   - IRI：<...> 或 prefix:local
//   - 字面量："..."、"""..."""、'...'，带 @lang 或 ^^<type>
//   - 关键字 a（等价 rdf:type）
//   - 语句分隔：. / ; / ,
//   - 注释 # 至行尾
//
// 不支持：blank node []、集合 ()、嵌套、复杂转义之外的高级特性。
// 这些对个人本体管理已足够；如需更复杂输入，可后续替换实现。
func ParseTurtle(src string) (triples []Triple, prefixes PrefixMap, err error) {
	p := &turtleParser{
		src: src,
		prefix: PrefixMap{
			"rdf":  RDF.Base(),
			"rdfs": RDFS.Base(),
			"owl":  OWL.Base(),
			"xsd":  XSD.Base(),
		},
	}
	return p.parse()
}

type turtleParser struct {
	src    string
	pos    int
	prefix PrefixMap
}

func (p *turtleParser) parse() ([]Triple, PrefixMap, error) {
	var triples []Triple
	for {
		p.skipWS()
		if p.eof() {
			break
		}

		// 前缀声明。
		if p.peekKW("@prefix") || p.peekKW("PREFIX") {
			if err := p.parsePrefixDecl(); err != nil {
				return nil, nil, err
			}
			continue
		}
		if p.peekKW("@base") || p.peekKW("BASE") {
			// 解析并忽略 base（本项目暂不使用相对 IRI）。
			if err := p.parseBaseDecl(); err != nil {
				return nil, nil, err
			}
			continue
		}

		// 一条三元组语句。
		ts, err := p.parseTriplesBlock()
		if err != nil {
			return nil, nil, err
		}
		triples = append(triples, ts...)
	}
	return triples, p.prefix, nil
}

// parsePrefixDecl 解析 @prefix name: <uri> .
func (p *turtleParser) parsePrefixDecl() error {
	// 消费 @prefix / PREFIX 关键字。
	if p.peekKW("@prefix") {
		p.pos += len("@prefix")
	} else {
		p.consumeKW("PREFIX")
	}
	p.skipInlineWS()

	// 前缀名（可能为空，表示 ":"）。
	name := ""
	if !p.eof() && p.src[p.pos] != ':' {
		start := p.pos
		for !p.eof() && p.src[p.pos] != ':' && !isWS(p.src[p.pos]) {
			p.pos++
		}
		name = p.src[start:p.pos]
	}
	if p.eof() || p.src[p.pos] != ':' {
		return p.errorf("expected ':' in @prefix declaration")
	}
	p.pos++ // 消费 ':'
	p.skipInlineWS()

	uri, err := p.parseIRIRef()
	if err != nil {
		return err
	}
	p.skipInlineWS()
	if err := p.expectDot(); err != nil {
		return err
	}
	p.prefix[name] = uri
	return nil
}

// parseBaseDecl 解析并丢弃 @base <uri> .
func (p *turtleParser) parseBaseDecl() error {
	if p.peekKW("@base") {
		p.pos += len("@base")
	} else {
		p.consumeKW("BASE")
	}
	p.skipInlineWS()
	if _, err := p.parseIRIRef(); err != nil {
		return err
	}
	p.skipInlineWS()
	return p.expectDot()
}

// parseTriplesBlock 解析 subject predicateObjectList '.' 语句。
func (p *turtleParser) parseTriplesBlock() ([]Triple, error) {
	p.skipInlineWS()
	subj, err := p.parseTerm(true)
	if err != nil {
		return nil, err
	}

	var triples []Triple
	for {
		p.skipInlineWS()
		pred, err := p.parsePredicate()
		if err != nil {
			return nil, err
		}

		// object list（逗号分隔）
		for {
			p.skipInlineWS()
			obj, err := p.parseTerm(false)
			if err != nil {
				return nil, err
			}
			triples = append(triples, Triple{Subject: subj, Predicate: pred, Object: obj})

			p.skipInlineWS()
			if p.eof() {
				return nil, p.errorf("unexpected EOF, expected '.' / ';' / ','")
			}
			c := p.src[p.pos]
			if c == ',' {
				p.pos++
				continue // 同 s+p 的下一个 object
			}
			break // 结束 object list
		}

		// 语句分隔。
		if p.eof() {
			return nil, p.errorf("unexpected EOF, expected '.' or ';'")
		}
		c := p.src[p.pos]
		switch c {
		case ';':
			p.pos++
			p.skipInlineWS()
			// ';' 后可能直接 '.'（允许末尾多余分号）。
			if !p.eof() && p.src[p.pos] == '.' {
				p.pos++
				return triples, nil
			}
			continue // 下一个 predicateObjectList
		case '.':
			p.pos++
			return triples, nil
		default:
			return nil, p.errorf("expected '.' or ';', got %q", string(c))
		}
	}
}

// parsePredicate 解析谓词位置，支持关键字 a。
func (p *turtleParser) parsePredicate() (Term, error) {
	p.skipInlineWS()
	if p.eof() {
		return Term{}, p.errorf("expected predicate")
	}
	// 关键字 a -> rdf:type
	if p.src[p.pos] == 'a' {
		next := byte(' ')
		if p.pos+1 < len(p.src) {
			next = p.src[p.pos+1]
		}
		if isWS(next) || next == '<' {
			p.pos++
			return Type, nil
		}
	}
	return p.parseIRI()
}

// parseTerm 解析 subject 或 object 位置的 Term。
// isSubject=true 时只接受 IRI/blank（subject 不能是 literal）。
func (p *turtleParser) parseTerm(isSubject bool) (Term, error) {
	p.skipInlineWS()
	if p.eof() {
		return Term{}, p.errorf("unexpected EOF parsing term")
	}
	c := p.src[p.pos]
	switch c {
	case '<':
		return p.parseIRIRefTerm()
	case '"', '\'':
		if isSubject {
			return Term{}, p.errorf("literal cannot be a subject")
		}
		return p.parseLiteral()
	case '_':
		return p.parseBlankNode()
	default:
		// prefix:local 形式
		return p.parsePrefixedOrKeyword(isSubject)
	}
}

// parseIRI 解析 IRI（谓词位置专用，支持 <...> 和 prefix:local）。
func (p *turtleParser) parseIRI() (Term, error) {
	p.skipInlineWS()
	if p.eof() {
		return Term{}, p.errorf("expected IRI")
	}
	if p.src[p.pos] == '<' {
		return p.parseIRIRefTerm()
	}
	return p.parsePrefixedOrKeyword(true)
}

// parseIRIRef 解析 <...> 内的 URI（纯字符串返回）。
func (p *turtleParser) parseIRIRef() (string, error) {
	t, err := p.parseIRIRefTerm()
	if err != nil {
		return "", err
	}
	return t.Value, nil
}

// parseIRIRefTerm 解析 <...> 形式的 IRI Term。
func (p *turtleParser) parseIRIRefTerm() (Term, error) {
	if p.eof() || p.src[p.pos] != '<' {
		return Term{}, p.errorf("expected '<'")
	}
	p.pos++ // 消费 '<'
	var b strings.Builder
	for !p.eof() {
		c := p.src[p.pos]
		if c == '>' {
			p.pos++
			return IRI(b.String()), nil
		}
		if c == '\\' && p.pos+1 < len(p.src) {
			// 简单转义：\<char>
			b.WriteByte(p.src[p.pos+1])
			p.pos += 2
			continue
		}
		b.WriteByte(c)
		p.pos++
	}
	return Term{}, p.errorf("unterminated IRI")
}

// parsePrefixedOrKeyword 解析 prefix:local 形式（或布尔值等关键字）。
func (p *turtleParser) parsePrefixedOrKeyword(isSubject bool) (Term, error) {
	start := p.pos
	// 读到 ':' 为止，收集前缀名。
	for !p.eof() && p.src[p.pos] != ':' {
		c := p.src[p.pos]
		if isWS(c) || c == '<' || c == '"' || c == '\'' || c == '.' || c == ';' || c == ',' {
			// 无冒号的关键字（如 true/false），只有 object 允许。
			break
		}
		p.pos++
	}
	if p.eof() {
		// 整段是裸词（非 subject）。
		word := p.src[start:p.pos]
		return p.asBareWord(word)
	}
	if p.src[p.pos] != ':' {
		// 没有 ':'，按裸词处理。
		word := p.src[start:p.pos]
		return p.asBareWord(word)
	}

	// prefix:local
	prefix := p.src[start:p.pos]
	p.pos++ // 消费 ':'
	localStart := p.pos
	for !p.eof() {
		c := p.src[p.pos]
		if isWS(c) || c == '<' || c == '"' || c == '\'' || c == '.' || c == ';' || c == ',' || c == '[' || c == ']' || c == '(' || c == ')' {
			break
		}
		p.pos++
	}
	local := p.src[localStart:p.pos]
	if local == "" {
		return Term{}, p.errorf("empty local name after %q:", prefix)
	}
	base, ok := p.prefix[prefix]
	if !ok {
		return Term{}, p.errorf("unknown prefix %q", prefix)
	}
	return IRI(base + local), nil
}

// asBareWord 把无冒号的裸词解释为 Term（仅支持 true/false/a 的简化）。
func (p *turtleParser) asBareWord(word string) (Term, error) {
	if word == "" {
		return Term{}, p.errorf("expected term")
	}
	if word == "true" {
		return BoolLit(true), nil
	}
	if word == "false" {
		return BoolLit(false), nil
	}
	return Term{}, p.errorf("unexpected token %q", word)
}

// parseLiteral 解析字符串字面量，含 @lang 与 ^^<type>。
func (p *turtleParser) parseLiteral() (Term, error) {
	val, err := p.parseString()
	if err != nil {
		return Term{}, err
	}
	// 检查语言标签或数据类型。
	p.skipInlineWS()
	if !p.eof() {
		c := p.src[p.pos]
		if c == '@' {
			p.pos++
			start := p.pos
			for !p.eof() {
				c := p.src[p.pos]
				if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
					break
				}
				p.pos++
			}
			return LangLit(val, p.src[start:p.pos]), nil
		}
		if c == '^' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '^' {
			p.pos += 2
			p.skipInlineWS()
			dtTerm, err := p.parseIRI()
			if err != nil {
				return Term{}, err
			}
			return TypedLit(val, dtTerm.Value), nil
		}
	}
	return Lit(val), nil
}

// parseString 解析 "..."、'...'、"""..."""、”'...”'。
func (p *turtleParser) parseString() (string, error) {
	if p.eof() {
		return "", p.errorf("expected string")
	}
	q := p.src[p.pos]
	if q != '"' && q != '\'' {
		return "", p.errorf("expected string quote, got %q", string(q))
	}
	// 三引号？
	triple := false
	if p.pos+2 < len(p.src) && p.src[p.pos+1] == q && p.src[p.pos+2] == q {
		triple = true
		p.pos += 3
	} else {
		p.pos++
	}

	var b strings.Builder
	for !p.eof() {
		c := p.src[p.pos]
		if c == '\\' && p.pos+1 < len(p.src) {
			n := p.src[p.pos+1]
			switch n {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '"':
				b.WriteByte('"')
			case '\'':
				b.WriteByte('\'')
			case '\\':
				b.WriteByte('\\')
			case 'u':
				if p.pos+5 < len(p.src) {
					r, err := parseHex4(p.src[p.pos+2 : p.pos+6])
					if err == nil {
						b.WriteRune(r)
						p.pos += 4
					}
				}
			default:
				b.WriteByte(n)
			}
			p.pos += 2
			continue
		}
		if triple {
			if p.pos+2 < len(p.src) && p.src[p.pos] == q && p.src[p.pos+1] == q && p.src[p.pos+2] == q {
				p.pos += 3
				return b.String(), nil
			}
			b.WriteByte(c)
			p.pos++
			continue
		}
		if c == q {
			p.pos++
			return b.String(), nil
		}
		b.WriteByte(c)
		p.pos++
	}
	return "", p.errorf("unterminated string")
}

// parseBlankNode 解析 _:label 形式的空白节点。
func (p *turtleParser) parseBlankNode() (Term, error) {
	if p.eof() || p.src[p.pos] != '_' {
		return Term{}, p.errorf("expected blank node")
	}
	p.pos++ // 消费 '_'
	if p.eof() || p.src[p.pos] != ':' {
		return Term{}, p.errorf("expected ':' in blank node")
	}
	p.pos++ // 消费 ':'
	start := p.pos
	for !p.eof() {
		c := p.src[p.pos]
		if isWS(c) || c == '.' || c == ';' || c == ',' || c == '[' || c == ']' {
			break
		}
		p.pos++
	}
	return Blank(p.src[start:p.pos]), nil
}

// --- 辅助 ---

func (p *turtleParser) expectDot() error {
	p.skipInlineWS()
	if p.eof() || p.src[p.pos] != '.' {
		return p.errorf("expected '.'")
	}
	p.pos++
	return nil
}

func (p *turtleParser) skipWS() {
	for !p.eof() {
		c := p.src[p.pos]
		if isWS(c) {
			p.pos++
			continue
		}
		if c == '#' {
			// 注释到行尾。
			for !p.eof() && p.src[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		break
	}
}

// skipInlineWS 跳过空白与注释（与 skipWS 相同，语义别名）。
func (p *turtleParser) skipInlineWS() { p.skipWS() }

func (p *turtleParser) eof() bool { return p.pos >= len(p.src) }
func (p *turtleParser) errorf(format string, args ...any) error {
	line, col := p.lineCol()
	return fmt.Errorf("turtle parse error at line %d col %d: %s", line, col, fmt.Sprintf(format, args...))
}

func (p *turtleParser) lineCol() (int, int) {
	line, col := 1, 1
	for i := 0; i < p.pos && i < len(p.src); i++ {
		if p.src[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

// peekKW 判断当前位置是否以指定关键字（大小写敏感，区分 @ 形式与 SPARQL 形式）开头，
// 且其后跟随空白（避免把 @prefixxxx 误判）。
func (p *turtleParser) peekKW(kw string) bool {
	if p.pos+len(kw) > len(p.src) {
		return false
	}
	if p.src[p.pos:p.pos+len(kw)] != kw {
		return false
	}
	after := p.pos + len(kw)
	if after < len(p.src) {
		c := p.src[after]
		if !isWS(c) {
			return false
		}
	}
	return true
}

// consumeKW 消费一个关键字（用于 SPARQL 风格的 PREFIX/BASE）。
func (p *turtleParser) consumeKW(kw string) {
	if p.peekKW(kw) {
		p.pos += len(kw)
	}
}

func isWS(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func parseHex4(s string) (rune, error) {
	var r rune
	for _, c := range s {
		r <<= 4
		switch {
		case c >= '0' && c <= '9':
			r += c - '0'
		case c >= 'a' && c <= 'f':
			r += c - 'a' + 10
		case c >= 'A' && c <= 'F':
			r += c - 'A' + 10
		default:
			return 0, fmt.Errorf("bad hex digit %q", string(c))
		}
	}
	return r, nil
}

// 确保 unicode 包未被标记为未使用（若未来移除会自动清理）。
var _ = unicode.IsLetter
