// Package web 实现 MyOntopo 的 Web UI 服务器。
//
// 提供：
//   - 交互式本体图可视化
//   - 推理规则浏览与启停控制
//   - 推理执行与结果展示
//   - 一致性检查
package web

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/walker/myonto/internal/reasoning"
	"github.com/walker/myonto/internal/rules"
	"github.com/walker/myonto/internal/store"
)

//go:embed ui/index.html
var embeddedIndexHTML []byte

// Config 是 Web 服务器配置。
type Config struct {
	Host string // 监听地址，默认 "localhost"
	Port int    // 端口，默认 7399
	Dir  string // 项目目录（用于查找外部模板）
}

// Server 是 MyOntopo Web UI 服务器。
type Server struct {
	store    *store.Store
	reasoner *reasoning.Reasoner
	engine   *rules.Engine
	cfg      Config
	listener net.Listener
}

// NewServer 创建 Web 服务器。
func NewServer(s *store.Store, cfg Config) *Server {
	if cfg.Port == 0 {
		cfg.Port = 7399
	}
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}

	// 创建推理器
	reasoner := reasoning.NewReasoner(s.Triples())

	return &Server{
		store:    s,
		reasoner: reasoner,
		engine:   reasoner.Engine(),
		cfg:      cfg,
	}
}

// Handler 返回 HTTP handler。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// SPA 首页
	mux.HandleFunc("/", s.handleIndex)

	// API 路由
	mux.HandleFunc("/api/rules", s.handleRules)
	mux.HandleFunc("/api/rules/", s.handleRuleToggle)
	mux.HandleFunc("/api/reason", s.handleReason)
	mux.HandleFunc("/api/check", s.handleCheck)
	mux.HandleFunc("/api/graph", s.handleGraph)
	mux.HandleFunc("/api/triples", s.handleTriples)
	mux.HandleFunc("/api/stats", s.handleStats)

	return mux
}

// Start 启动 HTTP 服务器并返回监听地址。
// 如果端口被占用且不是自己的进程，自动尝试下一个端口（最多试 10 次）。
func (s *Server) Start() (string, error) {
	maxRetry := 10
	for i := 0; i < maxRetry; i++ {
		addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			// 端口被占用，检查是否是自己的进程
			if isAddrInUse(err) {
				if isOwnServer(addr) {
					return "", fmt.Errorf("myonto web UI already running at http://%s", addr)
				}
				// 不是自己的，换一个端口
				log.Printf("port %d occupied, trying %d", s.cfg.Port, s.cfg.Port+1)
				s.cfg.Port++
				continue
			}
			return "", fmt.Errorf("listen %s: %w", addr, err)
		}
		s.listener = ln

		go func() {
			srv := &http.Server{Handler: s.Handler()}
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				log.Printf("web server error: %v", err)
			}
		}()

		return fmt.Sprintf("http://%s", ln.Addr().String()), nil
	}
	return "", fmt.Errorf("failed to find available port after %d retries", maxRetry)
}

// Stop 优雅关闭服务器。
func (s *Server) Stop(ctx context.Context) error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// loadIndexHTML 加载 index.html，优先从文件系统读取，否则用内嵌的。
func (s *Server) loadIndexHTML() []byte {
	// 1. 项目目录下的 .myonto/web/index.html
	if s.cfg.Dir != "" {
		path := filepath.Join(s.cfg.Dir, ".myonto", "web", "index.html")
		if data, err := os.ReadFile(path); err == nil {
			return data
		}
	}
	// 2. 用户全局配置
	home, _ := os.UserHomeDir()
	if home != "" {
		path := filepath.Join(home, ".config", "myonto", "web", "index.html")
		if data, err := os.ReadFile(path); err == nil {
			return data
		}
	}
	// 3. 内嵌默认
	return embeddedIndexHTML
}

// handleIndex 返回 SPA 页面。
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(s.loadIndexHTML())
}

// handleRules 返回所有规则信息。
func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defs := s.engine.Rules()
	infos := make([]rules.RuleInfo, len(defs))
	for i, d := range defs {
		infos[i] = d.ToInfo()
	}
	writeJSON(w, infos)
}

// handleRuleToggle 切换规则的启用/禁用状态。
func (s *Server) handleRuleToggle(w http.ResponseWriter, r *http.Request) {
	// 从路径提取规则 ID：/api/rules/{id}
	id := r.URL.Path[len("/api/rules/"):]
	if id == "" {
		http.Error(w, "missing rule id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		defs := s.engine.Rules()
		found := false
		for i, d := range defs {
			if d.ID == id {
				defs[i].Enabled = body.Enabled
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "rule not found", http.StatusNotFound)
			return
		}
		s.engine.UpdateRules(defs)
		writeJSON(w, map[string]any{"id": id, "enabled": body.Enabled})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleReason 执行推理并返回结果。
func (s *Server) handleReason(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result := s.engine.Derive(s.store.Triples())
	writeJSON(w, result)
}

// handleCheck 执行一致性检查。
func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	findings := s.reasoner.Check()
	writeJSON(w, map[string]any{"findings": findings})
}

// handleGraph 返回图数据。
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	nodes, edges := buildGraph(s.store)
	writeJSON(w, map[string]any{"nodes": nodes, "edges": edges})
}

// handleTriples 查询三元组。
func (s *Server) handleTriples(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	triples := queryTriples(s.store, q.Get("s"), q.Get("p"), q.Get("o"))
	writeJSON(w, triples)
}

// handleStats 返回统计信息。
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	derived, stats := s.reasoner.DeriveWithStats()
	writeJSON(w, map[string]any{
		"total_triples":    s.store.Len(),
		"inferred_triples": len(derived),
		"rule_stats":       stats,
		"timestamp":        time.Now().Format(time.RFC3339),
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
