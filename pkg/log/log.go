package log

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	defaultLogger *slog.Logger
	once          sync.Once
	rotateWriter  *DailyRotateWriter
)

// DailyRotateWriter 日志写入器
// 实时写入 logs/app.log，日期切换时归档到 logs/app-YYYY-MM-DD.log 并清空实时文件
type DailyRotateWriter struct {
	mu sync.Mutex

	// 实时日志 — 日常只写这一个文件
	realtimePath string   // logs/app.log
	realtimeFile *os.File

	// 归档配置
	dir     string // logs
	prefix  string // app
	ext     string // .log
	current string // 当前日期 YYYY-MM-DD
}

// NewDailyRotateWriter 创建日志写入器
func NewDailyRotateWriter(basePath string) (*DailyRotateWriter, error) {
	dir := filepath.Dir(basePath)
	base := filepath.Base(basePath)
	ext := filepath.Ext(base)
	prefix := strings.TrimSuffix(base, ext)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}

	w := &DailyRotateWriter{
		realtimePath: basePath,
		dir:          dir,
		prefix:       prefix,
		ext:          ext,
		current:      time.Now().Format("2006-01-02"),
	}

	f, err := os.OpenFile(basePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("打开实时日志文件失败: %w", err)
	}
	w.realtimeFile = f

	return w, nil
}

// archive 将当前实时日志归档到日期文件，然后清空实时文件
func (w *DailyRotateWriter) archive(today string) error {
	// 关闭实时文件（确保所有数据刷盘）
	w.realtimeFile.Close()

	// 读取实时日志内容
	data, err := os.ReadFile(w.realtimePath)
	if err != nil {
		// 读不到就重新打开，不阻塞
		f, _ := os.OpenFile(w.realtimePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		w.realtimeFile = f
		return fmt.Errorf("读取实时日志失败: %w", err)
	}

	// 写入归档文件
	archiveName := filepath.Join(w.dir, w.prefix+"-"+w.current+w.ext)
	if len(data) > 0 {
		af, err := os.OpenFile(archiveName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			af.Write(data)
			af.Close()
		}
	}

	// 清空实时文件，重新打开
	f, err := os.OpenFile(w.realtimePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("清空实时日志失败: %w", err)
	}
	w.realtimeFile = f
	w.current = today
	return nil
}

// Write 只写实时日志，日期变化时自动归档
func (w *DailyRotateWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if today != w.current {
		if err := w.archive(today); err != nil {
			slog.Error("日志归档失败", "err", err)
		}
	}

	if w.realtimeFile == nil {
		return 0, fmt.Errorf("日志文件未打开")
	}
	return w.realtimeFile.Write(p)
}

// Close 关闭实时日志文件
func (w *DailyRotateWriter) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.realtimeFile != nil {
		w.realtimeFile.Close()
	}
}

// ClearRealtime 清空实时日志文件
func (w *DailyRotateWriter) ClearRealtime() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.realtimeFile != nil {
		w.realtimeFile.Close()
		f, err := os.OpenFile(w.realtimePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}
		w.realtimeFile = f
	}
	return nil
}

// Init 初始化日志器。
// level: "debug", "info", "warn", "error"
// jsonMode: true 输出 JSON 格式
// filePath: 实时日志文件路径，为空则不写文件
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
		opts := &slog.HandlerOptions{Level: lvl, ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					if t, ok := a.Value.Any().(time.Time); ok {
						return slog.Attr{Key: a.Key, Value: slog.StringValue(t.Format("2006-01-02 15:04:05"))}
					}
				}
				return a
			}}

		var writers []io.Writer
		if console {
			writers = append(writers, os.Stdout)
		}
		if filePath != "" {
			w, err := NewDailyRotateWriter(filePath)
			if err != nil {
				slog.Error("日志初始化失败", "path", filePath, "err", err)
				if !console {
					writers = append(writers, os.Stdout)
				}
			} else {
				rotateWriter = w
				writers = append(writers, w)
			}
		}

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
	if rotateWriter != nil {
		rotateWriter.Close()
	}
}

// ClearLogs 清空实时日志文件
func ClearLogs() error {
	if rotateWriter != nil {
		return rotateWriter.ClearRealtime()
	}
	return nil
}

// Logger 返回默认日志器
func Logger() *slog.Logger {
	if defaultLogger == nil {
		Init("info", false, "", true)
	}
	return defaultLogger
}