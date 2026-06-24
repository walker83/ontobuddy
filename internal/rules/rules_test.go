package rules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefault(t *testing.T) {
	rules, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault() error: %v", err)
	}
	if len(rules) != 9 {
		t.Fatalf("expected 9 default rules, got %d", len(rules))
	}

	ids := make(map[string]bool)
	for _, r := range rules {
		if r.ID == "" {
			t.Errorf("rule missing ID: %+v", r)
		}
		if r.Name == "" {
			t.Errorf("rule %s missing Name", r.ID)
		}
		if ids[r.ID] {
			t.Errorf("duplicate rule ID: %s", r.ID)
		}
		ids[r.ID] = true
	}

	expected := []string{
		"subclass-transitive", "type-inheritance",
		"subproperty-transitive", "property-inheritance",
		"transitive-property", "symmetric-property", "inverse-of",
		"domain", "range",
	}
	for _, id := range expected {
		if !ids[id] {
			t.Errorf("missing expected rule: %s", id)
		}
	}
}

func TestLoadDefault_AllBuiltin(t *testing.T) {
	rules, _ := LoadDefault()
	for _, r := range rules {
		if r.Implementation != "builtin" {
			t.Errorf("rule %s: expected implementation 'builtin', got '%s'", r.ID, r.Implementation)
		}
	}
}

func TestLoadDefault_Categories(t *testing.T) {
	rules, _ := LoadDefault()
	rdfsCount, owlCount := 0, 0
	for _, r := range rules {
		switch r.Category {
		case "rdfs":
			rdfsCount++
		case "owl":
			owlCount++
		default:
			t.Errorf("rule %s: unexpected category '%s'", r.ID, r.Category)
		}
	}
	if rdfsCount == 0 {
		t.Error("expected some rdfs rules")
	}
	if owlCount == 0 {
		t.Error("expected some owl rules")
	}
}

func TestParseRules(t *testing.T) {
	yaml := `
rules:
  - id: test-rule
    name: "Test Rule"
    description: "A test rule"
    category: custom
    enabled: true
    spec:
      type: chain
      premises:
        - {s: "?x", p: "rdf:type", o: "?y"}
      conclusion: {s: "?x", p: "rdf:type", o: "?z"}
      filters:
        - "!= ?x ?z"
`
	rules, err := ParseRules([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseRules() error: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	r := rules[0]
	if r.ID != "test-rule" {
		t.Errorf("expected ID 'test-rule', got '%s'", r.ID)
	}
	if r.Spec.Type != "chain" {
		t.Errorf("expected spec type 'chain', got '%s'", r.Spec.Type)
	}
	if len(r.Spec.Premises) != 1 {
		t.Errorf("expected 1 premise, got %d", len(r.Spec.Premises))
	}
	if r.Spec.Conclusion == nil {
		t.Error("expected conclusion, got nil")
	}
	if len(r.Spec.Filters) != 1 {
		t.Errorf("expected 1 filter, got %d", len(r.Spec.Filters))
	}
}

func TestParseRules_DefaultImplementation(t *testing.T) {
	yaml := `
rules:
  - id: test
    name: Test
    spec:
      type: chain
`
	rules, err := ParseRules([]byte(yaml))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if rules[0].Implementation != "builtin" {
		t.Errorf("expected default implementation 'builtin', got '%s'", rules[0].Implementation)
	}
}

func TestParseRules_InvalidYAML(t *testing.T) {
	_, err := ParseRules([]byte("not: valid: yaml: ["))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestParseRulesValidation(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"missing id", `rules: [{name: x, spec: {type: chain}}]`, "missing id"},
		{"missing name", `rules: [{id: x, spec: {type: chain}}]`, "missing name"},
		{"missing type", `rules: [{id: x, name: x}]`, "missing spec.type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRules([]byte(tt.yaml))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got := err.Error(); !contains(got, tt.want) {
				t.Errorf("expected error containing %q, got %q", tt.want, got)
			}
		})
	}
}

// --- LoadFile ---

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.yaml")
	content := `
rules:
  - id: file-rule
    name: File Rule
    enabled: true
    spec:
      type: chain
`
	os.WriteFile(path, []byte(content), 0o644)

	rules, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != "file-rule" {
		t.Errorf("unexpected rules: %+v", rules)
	}
}

func TestLoadFile_NotFound(t *testing.T) {
	_, err := LoadFile("/nonexistent/path/rules.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// --- SaveRules ---

func TestSaveRules(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "rules.yaml")

	rules := []RuleDef{
		{ID: "saved", Name: "Saved Rule", Enabled: true, Spec: RuleSpec{Type: "chain"}},
	}
	if err := SaveRules(path, rules); err != nil {
		t.Fatalf("SaveRules() error: %v", err)
	}

	// 验证文件存在且可解析
	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error: %v", err)
	}
	if len(loaded) != 1 || loaded[0].ID != "saved" {
		t.Errorf("unexpected loaded rules: %+v", loaded)
	}
}

func TestSaveRules_ContainsHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.yaml")
	SaveRules(path, []RuleDef{{ID: "x", Name: "X", Spec: RuleSpec{Type: "chain"}}})

	data, _ := os.ReadFile(path)
	if !contains(string(data), "# MyOntopo") {
		t.Error("expected header comment in saved file")
	}
}

// --- LoadWithOverrides ---

func TestLoadWithOverrides_ProjectLocal(t *testing.T) {
	dir := t.TempDir()
	myontoDir := filepath.Join(dir, ".myonto")
	os.MkdirAll(myontoDir, 0o755)
	override := `
rules:
  - id: subclass-transitive
    name: "Custom SubClass"
    enabled: false
    spec:
      type: chain
`
	os.WriteFile(filepath.Join(myontoDir, "rules.yaml"), []byte(override), 0o644)

	rules, err := LoadWithOverrides(dir)
	if err != nil {
		t.Fatalf("LoadWithOverrides() error: %v", err)
	}

	// 找到 subclass-transitive，应该是被覆盖的
	for _, r := range rules {
		if r.ID == "subclass-transitive" {
			if r.Name != "Custom SubClass" {
				t.Errorf("expected overridden name, got '%s'", r.Name)
			}
			if r.Enabled {
				t.Error("expected disabled")
			}
			return
		}
	}
	t.Error("subclass-transitive not found")
}

func TestLoadWithOverrides_NoOverride(t *testing.T) {
	dir := t.TempDir()
	rules, err := LoadWithOverrides(dir)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(rules) != 9 {
		t.Errorf("expected 9 default rules, got %d", len(rules))
	}
}

// --- mergeRules ---

func TestMergeRules(t *testing.T) {
	base := []RuleDef{
		{ID: "a", Name: "A", Enabled: true},
		{ID: "b", Name: "B", Enabled: true},
	}
	override := []RuleDef{
		{ID: "b", Name: "B-override", Enabled: false},
		{ID: "c", Name: "C", Enabled: true},
	}
	merged := mergeRules(base, override)
	if len(merged) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(merged))
	}
	if merged[0].Name != "A" {
		t.Errorf("rule A should be unchanged, got %s", merged[0].Name)
	}
	if merged[1].Name != "B-override" {
		t.Errorf("rule B should be overridden, got %s", merged[1].Name)
	}
	if merged[1].Enabled {
		t.Error("rule B should be disabled")
	}
	if merged[2].ID != "c" {
		t.Errorf("rule C should be appended, got %s", merged[2].ID)
	}
}

func TestMergeRules_EmptyOverride(t *testing.T) {
	base := []RuleDef{{ID: "a", Name: "A"}}
	merged := mergeRules(base, nil)
	if len(merged) != 1 {
		t.Errorf("expected 1, got %d", len(merged))
	}
}

func TestMergeRules_EmptyBase(t *testing.T) {
	override := []RuleDef{{ID: "x", Name: "X"}}
	merged := mergeRules(nil, override)
	if len(merged) != 1 {
		t.Errorf("expected 1, got %d", len(merged))
	}
}

// --- ToInfo ---

func TestToInfo(t *testing.T) {
	r := RuleDef{
		ID:          "test",
		Name:        "Test",
		Description: "desc",
		Category:    "rdfs",
		Enabled:     true,
		Spec: RuleSpec{
			Type:       "chain",
			Premises:   []Pattern{{S: "?x", P: "rdf:type", O: "?y"}},
			Conclusion: &Pattern{S: "?x", P: "rdf:type", O: "?z"},
			Filters:    []string{"!= ?x ?z"},
		},
	}
	info := r.ToInfo()
	if info.ID != "test" {
		t.Errorf("expected ID 'test', got '%s'", info.ID)
	}
	if info.PremiseCount != 1 {
		t.Errorf("expected PremiseCount 1, got %d", info.PremiseCount)
	}
	if !info.HasConclusion {
		t.Error("expected HasConclusion true")
	}
	if info.FilterCount != 1 {
		t.Errorf("expected FilterCount 1, got %d", info.FilterCount)
	}
}

func TestToInfo_Builtin(t *testing.T) {
	r := RuleDef{
		ID:   "builtin-test",
		Name: "Builtin",
		Spec: RuleSpec{
			Type:        "builtin",
			BuiltinID:   "transitive-property",
			BuiltinDesc: "BFS closure",
		},
	}
	info := r.ToInfo()
	if info.SpecType != "builtin" {
		t.Errorf("expected 'builtin', got '%s'", info.SpecType)
	}
	if info.HasConclusion {
		t.Error("builtin should not have conclusion")
	}
}

// --- GetBuiltinFunc ---

func TestGetBuiltinFunc_AllExist(t *testing.T) {
	ids := []string{
		"subclass-transitive", "type-inheritance",
		"subproperty-transitive", "property-inheritance",
		"transitive-property", "symmetric-property", "inverse-of",
		"domain", "range",
	}
	for _, id := range ids {
		fn, ok := GetBuiltinFunc(id)
		if !ok {
			t.Errorf("missing builtin: %s", id)
		}
		if fn == nil {
			t.Errorf("nil func for: %s", id)
		}
	}
}

func TestGetBuiltinFunc_NotFound(t *testing.T) {
	_, ok := GetBuiltinFunc("nonexistent")
	if ok {
		t.Error("expected false for nonexistent builtin")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstr(s, sub))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
