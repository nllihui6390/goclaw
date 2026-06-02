package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Config 服务器配置
type Config struct {
	Port      string
	AuthToken string
}

// Server HTTP 服务器（管理 API + 前端 SPA）
type Server struct {
	cfg    Config
	mux    *http.ServeMux
	httpSrv *http.Server
}

// New 创建服务器
func New(cfg Config) *Server {
	s := &Server{cfg: cfg}
	s.setupRoutes()
	return s
}

// Mux 返回 ServeMux，供外部（如 gateway）追加路由
func (s *Server) Mux() *http.ServeMux {
	return s.mux
}

// Start 启动服务器
func (s *Server) Start() error {
	s.httpSrv = &http.Server{
		Addr:    ":" + s.cfg.Port,
		Handler: s.authMiddleware(s.mux),
	}
	go func() {
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	return nil
}

func (s *Server) setupRoutes() {
	mux := http.NewServeMux()

	// 管理 API
	mux.HandleFunc("/api/v1/agents", handleAgents)
	mux.HandleFunc("/api/v1/agents/", handleAgentByID)
	mux.HandleFunc("/api/v1/channels", handleChannels)
	mux.HandleFunc("/api/v1/channels/", handleChannelByID)
	mux.HandleFunc("/api/v1/providers", handleProviders)
	mux.HandleFunc("/api/v1/tools", handleTools)
	mux.HandleFunc("/api/v1/skills", handleSkills)
	mux.HandleFunc("/api/v1/cron/jobs", handleCronJobs)
	mux.HandleFunc("/api/v1/cron/jobs/", handleCronJobByID)
	mux.HandleFunc("/api/v1/config", handleConfig)
	mux.HandleFunc("/api/v1/config/reload", handleConfigReload)
	mux.HandleFunc("/api/v1/logs", handleLogs)
	mux.HandleFunc("/api/v1/status", handleStatus)
	mux.HandleFunc("/api/v1/sessions", handleSessions)
	mux.HandleFunc("/api/v1/sessions/", handleSessionByID)
	mux.HandleFunc("/api/v1/agent-files/", handleAgentFiles)

	// 前端 SPA
	mux.HandleFunc("/", serveFrontend)

	s.mux = mux
}

// CORS + Auth 中间件
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Access-Control-Allow-Origin", "*")
		rw.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		rw.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")

		if r.Method == http.MethodOptions {
			rw.WriteHeader(http.StatusOK)
			return
		}

		// Auth (仅对 /api/ 路径)
		if s.cfg.AuthToken != "" && strings.HasPrefix(r.URL.Path, "/api/") {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != s.cfg.AuthToken {
				http.Error(rw, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}

		next.ServeHTTP(rw, r)
	})
}

// writeJSON 写入 JSON 响应
func writeJSON(rw http.ResponseWriter, status int, data any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	json.NewEncoder(rw).Encode(data)
}

// writeError 写入 JSON 错误响应
func writeError(rw http.ResponseWriter, status int, msg string) {
	writeJSON(rw, status, map[string]string{"error": msg})
}

var startTime = time.Now()
