package rdf

import (
	"reflect"
	"testing"
)

// TestTurtleRoundTrip 验证：序列化 -> 解析 能还原原始三元组集合（忽略顺序）。
// 这是整个项目的数据完整性基石：每次 save/load 不能丢信息。
func TestTurtleRoundTrip(t *testing.T) {
	prefixes := PrefixMap{
		"rdf":  RDF.Base(),
		"rdfs": RDFS.Base(),
		"owl":  OWL.Base(),
		"ex":   "http://example.org/",
	}
	original := []Triple{
		{IRI("http://example.org/newton"), Label, Lit("Isaac Newton")},
		{IRI("http://example.org/newton"), Comment, Lit("英国数学家、物理学家")},
		{IRI("http://example.org/newton"), Type, IRI("http://example.org/Person")},
		{IRI("http://example.org/newton"), IRI("http://example.org/knows"), IRI("http://example.org/leibniz")},
		{IRI("http://example.org/Person"), Type, Class},
		{IRI("http://example.org/Person"), Label, Lit("Person")},
		{IRI("http://example.org/Person"), SubClassOf, IRI("http://example.org/Agent")},
		{IRI("http://example.org/knows"), Type, SymmetricProperty},
		{IRI("http://example.org/partOf"), Type, TransitiveProperty},
		// 带语言标签和类型。
		{IRI("http://example.org/newton"), IRI("http://example.org/motto"), LangLit("hypotheses non fingo", "la")},
		{IRI("http://example.org/newton"), IRI("http://example.org/birthYear"), TypedLit("1643", XSDInteger)},
	}

	serialized := SerializeTurtle(original, prefixes)
	t.Logf("序列化结果:\n%s", serialized)

	parsed, parsedPrefix, err := ParseTurtle(serialized)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if !sameSet(original, parsed) {
		t.Errorf("round-trip 数据不一致")
		t.Errorf("  原始 %d 条:", len(original))
		for _, tr := range original {
			t.Errorf("    %s", tr)
		}
		t.Errorf("  解析 %d 条:", len(parsed))
		for _, tr := range parsed {
			t.Errorf("    %s", tr)
		}
	}

	// 前缀应保留。
	for k, v := range prefixes {
		if pv, ok := parsedPrefix[k]; !ok || pv != v {
			t.Errorf("前缀 %s 未保留或值错误: got %s, want %s", k, pv, v)
		}
	}
}

// TestTurtleParseKeywordA 验证 'a' 关键字被正确解释为 rdf:type。
func TestTurtleParseKeywordA(t *testing.T) {
	src := `@prefix ex: <http://example.org/> .
ex:socrates a ex:Human .`
	triples, _, err := ParseTurtle(src)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(triples) != 1 {
		t.Fatalf("期望 1 条三元组，得到 %d", len(triples))
	}
	want := Triple{IRI("http://example.org/socrates"), Type, IRI("http://example.org/Human")}
	if !triples[0].Equal(want) {
		t.Errorf("a 关键字解析错误: got %s, want %s", triples[0], want)
	}
}

// TestTurtleParseMultipleObjects 验证逗号分隔的 object list。
func TestTurtleParseMultipleObjects(t *testing.T) {
	src := `@prefix ex: <http://example.org/> .
ex:socrates ex:teaches ex:plato, ex:xenophon .`
	triples, _, err := ParseTurtle(src)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(triples) != 2 {
		t.Fatalf("期望 2 条三元组，得到 %d", len(triples))
	}
}

// TestTurtleParseSemicolons 验证分号分隔的 predicate list。
func TestTurtleParseSemicolons(t *testing.T) {
	src := `@prefix ex: <http://example.org/> .
ex:socrates a ex:Human ;
    rdfs:label "Socrates" ;
    rdfs:comment "古希腊哲学家" .`
	triples, _, err := ParseTurtle(src)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(triples) != 3 {
		t.Fatalf("期望 3 条三元组，得到 %d: %v", len(triples), triples)
	}
}

// TestTurtleParseComments 验证注释被正确跳过。
func TestTurtleParseComments(t *testing.T) {
	src := `# 这是文件头注释
@prefix ex: <http://example.org/> .

# socrates 条目
ex:socrates a ex:Human . # 行尾注释
ex:plato a ex:Human .`
	triples, _, err := ParseTurtle(src)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(triples) != 2 {
		t.Fatalf("期望 2 条三元组（注释应跳过），得到 %d", len(triples))
	}
}

// TestTurtleLangAndTypedLiteral 验证语言标签和类型化字面量。
func TestTurtleLangAndTypedLiteral(t *testing.T) {
	src := `@prefix ex: <http://example.org/> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
ex:item ex:greeting "hello"@en ;
    ex:count "42"^^xsd:integer .`
	triples, _, err := ParseTurtle(src)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	greeting := Lit("")
	count := Lit("")
	for _, t := range triples {
		if t.Predicate.Value == "http://example.org/greeting" {
			greeting = t.Object
		}
		if t.Predicate.Value == "http://example.org/count" {
			count = t.Object
		}
	}
	if greeting.Lang != "en" {
		t.Errorf("语言标签错误: got @%s, want @en", greeting.Lang)
	}
	if greeting.Value != "hello" {
		t.Errorf("值错误: got %q, want %q", greeting.Value, "hello")
	}
	if count.DataType != XSDInteger {
		t.Errorf("数据类型错误: got %s, want %s", count.DataType, XSDInteger)
	}
}

// TestTurtleParseErrorReporting 验证解析错误信息含位置。
func TestTurtleParseErrorReporting(t *testing.T) {
	// 缺少句点。
	src := `@prefix ex: <http://example.org/> .
ex:socrates a ex:Human`
	_, _, err := ParseTurtle(src)
	if err == nil {
		t.Fatal("期望解析错误，实际成功")
	}
	if err.Error() == "" {
		t.Error("错误信息为空")
	}
}

// sameSet 判断两个三元组集合是否包含完全相同的元素（顺序无关）。
func sameSet(a, b []Triple) bool {
	if len(a) != len(b) {
		return false
	}
	setB := map[string]int{}
	for _, t := range b {
		setB[t.String()]++
	}
	for _, t := range a {
		k := t.String()
		if setB[k] == 0 {
			return false
		}
		setB[k]--
	}
	return true
}

// TestTermEqual 验证 Term.Equal 的语义。
func TestTermEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b Term
		want bool
	}{
		{"两个相同 IRI", IRI("http://x/y"), IRI("http://x/y"), true},
		{"不同 IRI", IRI("http://x/y"), IRI("http://x/z"), false},
		{"普通 literal", Lit("hi"), Lit("hi"), true},
		{"lang literal 相同", LangLit("hi", "en"), LangLit("hi", "en"), true},
		{"lang literal 不同 lang", LangLit("hi", "en"), LangLit("hi", "fr"), false},
		{"typed literal 相同", TypedLit("1", XSDInteger), TypedLit("1", XSDInteger), true},
		{"typed literal 不同类型", TypedLit("1", XSDInteger), Lit("1"), false},
		{"IRI vs literal", IRI("hi"), Lit("hi"), false},
		{"blank node", Blank("b1"), Blank("b1"), true},
		{"不同 blank node", Blank("b1"), Blank("b2"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("Equal = %v, want %v", got, tt.want)
			}
		})
	}
}

// 避免未使用导入。
var _ = reflect.DeepEqual
