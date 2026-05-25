package middleware

import (
	"net/http"
	"strings"

	"go-claw/pkg/log"
)

// AuthMiddleware 验证Bearer Token鉴权中间件
func AuthMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, `{"error":"missing authorization"}`, http.StatusUnauthorized)
				return
			}

			if strings.TrimPrefix(auth, "Bearer ") != token {
				http.Error(w, `{"error":"invalid token"}`, http.StatusForbidden)
				log.Logger().Warn("鉴权失败", "ip", r.RemoteAddr)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
