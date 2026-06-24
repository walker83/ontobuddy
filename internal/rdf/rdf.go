// Package rdf 定义本项目的 RDF 三元组数据模型。
//
// 本项目自实现轻量 Turtle 读写层（不依赖外部 RDF 库），
// 因此 Term 用一个统一的接口表示 IRI / Literal / BlankNode，
// 便于 Turtle 解析器、Store、可视化层共享同一套类型。
package rdf

import (
	"fmt"
	"strconv"
	"strings"
)

// TermKind 标识 Term 的种类。
type TermKind int

const (
	KindIRI TermKind = iota
	KindLiteral
	KindBlank
)

// Term 是 RDF 图中任意节点/值的统一表示。
type Term struct {
	Kind     TermKind
	Value    string // IRI 的完整 URI；Literal 的词法形式；Blank 的内部 id
	Lang     string // 仅 Literal：语言标签，如 "zh"
	DataType string // 仅 Literal：数据类型 IRI，缺省为 xsd:string
}

// IRI 构造一个 IRI Term。
func IRI(uri string) Term {
	return Term{Kind: KindIRI, Value: uri}
}

// Lit 构造一个普通字符串 Literal（xsd:string）。
func Lit(s string) Term {
	return Term{Kind: KindLiteral, Value: s, DataType: XSDString}
}

// LangLit 构造带语言标签的 Literal。
func LangLit(s, lang string) Term {
	return Term{Kind: KindLiteral, Value: s, Lang: lang}
}

// TypedLit 构造带数据类型的 Literal。
func TypedLit(s, dt string) Term {
	return Term{Kind: KindLiteral, Value: s, DataType: dt}
}

// Blank 构造一个空白节点 Term。
func Blank(id string) Term {
	return Term{Kind: KindBlank, Value: id}
}

// Equal 判断两个 Term 是否完全相同。
func (t Term) Equal(o Term) bool {
	return t.Kind == o.Kind &&
		t.Value == o.Value &&
		t.Lang == o.Lang &&
		t.DataType == o.DataType
}

// String 返回 Term 的规范字符串表示（用于哈希/去重/调试）。
func (t Term) String() string {
	switch t.Kind {
	case KindIRI:
		return "<" + t.Value + ">"
	case KindBlank:
		return "_:" + t.Value
	case KindLiteral:
		var b strings.Builder
		b.WriteByte('"')
		b.WriteString(escapeLiteral(t.Value))
		b.WriteByte('"')
		if t.Lang != "" {
			b.WriteByte('@')
			b.WriteString(t.Lang)
		} else if t.DataType != "" && t.DataType != XSDString {
			b.WriteString("^^<")
			b.WriteString(t.DataType)
			b.WriteByte('>')
		}
		return b.String()
	}
	return ""
}

// LocalName 返回 IRI 的 local name（最后一个 # 或 / 之后的部分）。
// 非 IRI 返回原 Value。
func (t Term) LocalName() string {
	if t.Kind != KindIRI {
		return t.Value
	}
	v := t.Value
	if i := strings.LastIndexAny(v, "#/"); i >= 0 {
		return v[i+1:]
	}
	return v
}

// escapeLiteral 转义 Turtle 字符串字面量中的特殊字符。
func escapeLiteral(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Triple 是一条 RDF 三元组。
type Triple struct {
	Subject   Term
	Predicate Term
	Object    Term
}

// String 返回 Turtle 风格的三元组表示。
func (t Triple) String() string {
	return fmt.Sprintf("%s %s %s .", t.Subject, t.Predicate, t.Object)
}

// Equal 判断两条三元组是否完全相同（S/P/O 全等）。
func (t Triple) Equal(o Triple) bool {
	return t.Subject.Equal(o.Subject) &&
		t.Predicate.Equal(o.Predicate) &&
		t.Object.Equal(o.Object)
}

// --- 常用命名空间 ---

type ns struct{ base string }

// XSD：XML Schema 数据类型。
var XSD = ns{"http://www.w3.org/2001/XMLSchema#"}
var RDF = ns{"http://www.w3.org/1999/02/22-rdf-syntax-ns#"}
var RDFS = ns{"http://www.w3.org/2000/01/rdf-schema#"}
var OWL = ns{"http://www.w3.org/2002/07/owl#"}

// 常用的 XSD 数据类型字符串常量，避免到处拼接。
const (
	XSDString  = "http://www.w3.org/2001/XMLSchema#string"
	XSDInteger = "http://www.w3.org/2001/XMLSchema#integer"
	XSDBoolean = "http://www.w3.org/2001/XMLSchema#boolean"
)

// 常用谓词的预构造 IRI，避免到处散写字符串。
var (
	Type          = RDF.IRI("type")
	Label         = RDFS.IRI("label")
	Comment       = RDFS.IRI("comment")
	SubClassOf    = RDFS.IRI("subClassOf")
	SubPropertyOf = RDFS.IRI("subPropertyOf")
	Class         = RDFS.IRI("Class")
	Domain        = RDFS.IRI("domain")
	Range         = RDFS.IRI("range")

	OwlClass           = OWL.IRI("Class")
	OwlObjectProperty  = OWL.IRI("ObjectProperty")
	TransitiveProperty = OWL.IRI("TransitiveProperty")
	SymmetricProperty  = OWL.IRI("SymmetricProperty")
	InverseOf          = OWL.IRI("inverseOf")
	DisjointWith       = OWL.IRI("disjointWith")
	EquivalentClass    = OWL.IRI("equivalentClass")
)

// IRI 返回该命名空间下指定 local name 的 IRI Term。
func (n ns) IRI(local string) Term {
	return Term{Kind: KindIRI, Value: n.base + local}
}

// Base 返回命名空间的基础 URI。
func (n ns) Base() string { return n.base }

// --- 数值字面量辅助 ---

// IntLit 构造 xsd:integer Literal。
func IntLit(n int64) Term {
	return Term{Kind: KindLiteral, Value: strconv.FormatInt(n, 10), DataType: XSDInteger}
}

// BoolLit 构造 xsd:boolean Literal。
func BoolLit(b bool) Term {
	v := "false"
	if b {
		v = "true"
	}
	return Term{Kind: KindLiteral, Value: v, DataType: XSDBoolean}
}
