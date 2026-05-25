package log

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

var (
	defaultLogger *slog.Logger
	once          sync.Once
	fileWriter    *os.File // 日志文件句柄，用于关闭
)

// Init 初始化日志器。
// level: "debug", "info", "warn", "error"
// jsonMode: true 输出 JSON 格式
// filePath: 日志文件路径，为空则不写文件
// console: 是否同时输出到控制台
func Init(level string, jsonMode bool, filePath string, console bool) {
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

		// 构建输出目标
		var writers []io.Writer
		if console {
			writers = append(writers, os.Stdout)
		}
		if filePath != "" {
			// 自动创建日志目录
			if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
				slog.Error("日志目录创建失败", "dir", filepath.Dir(filePath), "err", err)
			}
			f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				// 文件打开失败，降级到只输出控制台
				slog.Error("日志文件打开失败", "path", filePath, "err", err)
				if !console {
					writers = append(writers, os.Stdout) // 保底：至少有输出
				}
			} else {
				fileWriter = f
				writers = append(writers, f)
			}
		}

		// 如果没有配置任何输出，至少输出到控制台
		if len(writers) == 0 {
			writers = append(writers, os.Stdout)
		}

		var dest io.Writer
		if len(writers) == 1 {
			dest = writers[0]
		} else {
			dest = io.MultiWriter(writers...)
		}

		var h slog.Handler
		if jsonMode {
			h = slog.NewJSONHandler(dest, opts)
		} else {
			h = slog.NewTextHandler(dest, opts)
		}
		defaultLogger = slog.New(h)
	})
}

// Close 关闭日志文件
func Close() {
	if fileWriter != nil {
		fileWriter.Close()
	}
}

// Logger 返回默认日志器
func Logger() *slog.Logger {
	if defaultLogger == nil {
		Init("info", false, "", true)
	}
	return defaultLogger
}