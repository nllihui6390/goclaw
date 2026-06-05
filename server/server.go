package server

import (
	"net/http"
	"strings"

	"go-claw/server/controllers/api"
)

// Config 服务器配置
type Config struct {
	Port      string
	AuthToken string
}

// Server HTTP 服务器（管理 API + 前端 SPA）
type Server struct {
	cfg     Config
	mux     *http.ServeMux
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
	// 初始化 service 层
	api.InitServices()

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

	// 管理 API - 使用 controllers/api 层
	mux.HandleFunc("/api/v1/agents", api.HandleAgents)
	mux.HandleFunc("/api/v1/agents/", api.HandleAgentByID)
	mux.HandleFunc("/api/v1/channels", api.HandleChannels)
	mux.HandleFunc("/api/v1/channels/", api.HandleChannelByID)
	mux.HandleFunc("/api/v1/channels/qrcode", api.HandleChannelQRCode)
	mux.HandleFunc("/api/v1/channels/qrcode/status", api.HandleChannelQRCodeStatus)
	mux.HandleFunc("/api/v1/providers", api.HandleProviders)
	mux.HandleFunc("/api/v1/tools", api.HandleTools)
	mux.HandleFunc("/api/v1/skills/pool", api.HandleSkillPool)
	mux.HandleFunc("/api/v1/skills/scan", api.HandleSkillScan)
	mux.HandleFunc("/api/v1/skills/upload", api.HandleSkillUpload)
	mux.HandleFunc("/api/v1/skills/enabled", api.HandleSkillEnabled)
	mux.HandleFunc("/api/v1/cron/jobs", api.HandleCronJobs)
	mux.HandleFunc("/api/v1/cron/jobs/", api.HandleCronJobByID)
	mux.HandleFunc("/api/v1/config", api.HandleConfig)
	mux.HandleFunc("/api/v1/config/reload", api.HandleConfigReload)
	mux.HandleFunc("/api/v1/restart", api.HandleRestart)
	mux.HandleFunc("/api/v1/logs", api.HandleLogs)
	mux.HandleFunc("/api/v1/status", api.HandleStatus)
	mux.HandleFunc("/api/v1/sessions", api.HandleSessions)
	mux.HandleFunc("/api/v1/sessions/", api.HandleSessionByID)
	mux.HandleFunc("/api/v1/agent-files/", api.HandleAgentFiles)
	mux.HandleFunc("/api/v1/files/download", api.HandleFileDownload)

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
