// Package store 是本体数据的内存存储层。
//
// 它封装一组去重的 Triple，提供按模式 (S/P/O) 查询的能力，
// 并负责 Turtle 文件的加载与保存。Store 不持久化任何索引，
// 完全在内存中工作——个人本体规模（通常 <10 万三元组）下足够快。
package store

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/walker/myonto/internal/config"
	"github.com/walker/myonto/internal/rdf"
)

// Store 是一个内存中的 RDF 图。
type Store struct {
	triples []rdf.Triple
	seen    map[string]struct{} // 三元组规范字符串 -> 存在标记，用于去重
	prefix  rdf.PrefixMap
	cfg     config.Config
}

// New 用给定配置构造一个空 Store。
func New(cfg config.Config) *Store {
	return &Store{
		prefix: defaultPrefixes(cfg),
		cfg:    cfg,
		seen:   map[string]struct{}{},
	}
}

// defaultPrefixes 构造序列化时使用的前缀表，包含标准词表与本地命名空间。
func defaultPrefixes(cfg config.Config) rdf.PrefixMap {
	p := rdf.PrefixMap{
		"rdf":  rdf.RDF.Base(),
		"rdfs": rdf.RDFS.Base(),
		"owl":  rdf.OWL.Base(),
		"xsd":  rdf.XSD.Base(),
	}
	if cfg.Prefix != "" && cfg.BaseIRI != "" {
		p[cfg.Prefix] = cfg.BaseIRI
	}
	return p
}

// LoadFile 从 Turtle 文件加载三元组到 Store（追加，不去重已有）。
func (s *Store) LoadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 空文件视为空图，不报错
		}
		return err
	}
	triples, prefixes, err := rdf.ParseTurtle(string(data))
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	for k, v := range prefixes {
		s.prefix[k] = v
	}
	for _, t := range triples {
		s.Add(t)
	}
	return nil
}

// SaveFile 把 Store 序列化为 Turtle 写入文件。
func (s *Store) SaveFile(path string) error {
	out := rdf.SerializeTurtle(s.triples, s.prefix)
	return os.WriteFile(path, []byte(out), 0o644)
}

// Add 加入一条三元组，若已存在则跳过。
// subject/object 若是字符串形式（如本地名），应由上层先转成 Term。
func (s *Store) Add(t rdf.Triple) {
	key := tripleKey(t)
	if _, ok := s.seen[key]; ok {
		return
	}
	s.seen[key] = struct{}{}
	s.triples = append(s.triples, t)
}

// Remove 删除所有与 pattern 完全匹配的三元组。
// 任一位置为零值 Term 表示通配（不限制）。
// 返回实际删除的数量。
func (s *Store) Remove(pattern rdf.Triple) int {
	// 用新 slice 收集保留的三元组，避免在遍历 s.triples 的同时原地覆写它
	// （s.triples[:0] 复用底层数组会与 range 的读指针产生别名，是脆弱写法，
	// 当前虽恰好正确但任何调整都可能引入难查的 bug）。
	out := make([]rdf.Triple, 0, len(s.triples))
	removed := 0
	for _, t := range s.triples {
		if matchPattern(t, pattern) {
			delete(s.seen, tripleKey(t))
			removed++
			continue
		}
		out = append(out, t)
	}
	s.triples = out
	return removed
}

// Has 判断某条三元组是否已存在。
func (s *Store) Has(t rdf.Triple) bool {
	_, ok := s.seen[tripleKey(t)]
	return ok
}

// Len 返回三元组数量。
func (s *Store) Len() int { return len(s.triples) }

// Triples 返回全部三元组的副本（按稳定顺序）。
func (s *Store) Triples() []rdf.Triple {
	out := make([]rdf.Triple, len(s.triples))
	copy(out, s.triples)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})
	return out
}

// Query 按模式查询三元组。零值 Term 表示通配。
func (s *Store) Query(pattern rdf.Triple) []rdf.Triple {
	var out []rdf.Triple
	for _, t := range s.triples {
		if matchPattern(t, pattern) {
			out = append(out, t)
		}
	}
	return out
}

// Subjects 返回图中所有出现过的 subject IRI Term（去重、排序）。
func (s *Store) Subjects() []rdf.Term {
	set := map[rdf.Term]struct{}{}
	for _, t := range s.triples {
		if t.Subject.Kind == rdf.KindIRI {
			set[t.Subject] = struct{}{}
		}
	}
	out := make([]rdf.Term, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

// Prefixes 返回前缀映射的副本。
func (s *Store) Prefixes() rdf.PrefixMap {
	out := make(rdf.PrefixMap, len(s.prefix))
	for k, v := range s.prefix {
		out[k] = v
	}
	return out
}

// AddPrefixes 把给定前缀合并到 Store 的前缀映射中。
func (s *Store) AddPrefixes(m rdf.PrefixMap) {
	for k, v := range m {
		s.prefix[k] = v
	}
}

// Config 返回关联的配置。
func (s *Store) Config() config.Config { return s.cfg }

// LocalIRI 把本地名（slug）转换成本命名空间下的完整 IRI Term。
func (s *Store) LocalIRI(local string) rdf.Term {
	return rdf.IRI(s.cfg.BaseIRI + local)
}

// ResolveName 把用户输入的名字解析为 IRI Term。
// 支持三种形式：
//   - "prefix:local"（用前缀表解析）
//   - 完整 "<http://...>"（保留尖括号或不含都行）
//   - 裸 local name（按本地命名空间解析）
func (s *Store) ResolveName(name string) (rdf.Term, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return rdf.Term{}, fmt.Errorf("empty name")
	}
	// 带尖括号的完整 IRI。
	if strings.HasPrefix(name, "<") && strings.HasSuffix(name, ">") {
		return rdf.IRI(name[1 : len(name)-1]), nil
	}
	if strings.Contains(name, "://") {
		return rdf.IRI(name), nil
	}
	// prefix:local
	if i := strings.Index(name, ":"); i > 0 {
		prefix, local := name[:i], name[i+1:]
		if base, ok := s.prefix[prefix]; ok {
			return rdf.IRI(base + local), nil
		}
		return rdf.Term{}, fmt.Errorf("unknown prefix %q", prefix)
	}
	// 裸 local name：走本地命名空间。
	return s.LocalIRI(name), nil
}

// --- 内部 ---

func tripleKey(t rdf.Triple) string {
	return t.String()
}

// matchPattern 判断三元组是否匹配模式；零值 Term 视为通配。
func matchPattern(t, pattern rdf.Triple) bool {
	if !isZero(pattern.Subject) && !t.Subject.Equal(pattern.Subject) {
		return false
	}
	if !isZero(pattern.Predicate) && !t.Predicate.Equal(pattern.Predicate) {
		return false
	}
	if !isZero(pattern.Object) && !t.Object.Equal(pattern.Object) {
		return false
	}
	return true
}

func isZero(t rdf.Term) bool { return t == (rdf.Term{}) }
