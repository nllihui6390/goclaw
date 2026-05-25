package log

import (
	"log/slog"
	"os"
	"sync"
)

var (
	defaultLogger *slog.Logger
	once          sync.Once
)

// Init 初始化日志器。level: "debug", "info", "warn", "error"。jsonMode=true 输出 JSON 格式。
func Init(level string, jsonMode bool) {
	once.Do(func() {
		lvl := slog.LevelInfo
		switch level {
		case "debug":
			lvl = slog.LevelDebug
		case "warn":
			lvl = slog.LevelWarn
		case "error":
			lvl = slog.LevelError
		}
		opts := &slog.HandlerOptions{Level: lvl}
		var h slog.Handler
		if jsonMode {
			h = slog.NewJSONHandler(os.Stdout, opts)
		} else {
			h = slog.NewTextHandler(os.Stdout, opts)
		}
		defaultLogger = slog.New(h)
	})
}

// Logger 返回默认日志器
func Logger() *slog.Logger {
	if defaultLogger == nil {
		Init("info", false)
	}
	return defaultLogger
}
