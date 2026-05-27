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

// DailyRotateWriter 按日期自动分割日志文件
// 文件名格式: basePath去掉扩展名 + "-YYYY-MM-DD" + 扩展名
// 例如: logs/app.log → logs/app-2026-05-27.log
type DailyRotateWriter struct {
	mu       sync.Mutex
	basePath string   // 原始配置路径，如 logs/app.log
	dir      string   // 目录，如 logs
	prefix   string   // 文件名前缀，如 app
	ext      string   // 扩展名，如 .log
	current  string   // 当前日期字符串 YYYY-MM-DD
	file     *os.File // 当前文件句柄
}

// NewDailyRotateWriter 创建按日期分割的日志写入器
func NewDailyRotateWriter(basePath string) (*DailyRotateWriter, error) {
	dir := filepath.Dir(basePath)
	base := filepath.Base(basePath)
	ext := filepath.Ext(base) // .log
	prefix := strings.TrimSuffix(base, ext) // app

	// 确保目录存在
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}

	w := &DailyRotateWriter{
		basePath: basePath,
		dir:      dir,
		prefix:   prefix,
		ext:      ext,
	}

	// 打开当天文件
	if err := w.rotate(); err != nil {
		return nil, err
	}

	return w, nil
}

// todayFileName 生成当天的日志文件名
func (w *DailyRotateWriter) todayFileName() string {
	return filepath.Join(w.dir, w.prefix+"-"+time.Now().Format("2006-01-02")+w.ext)
}

// rotate 切换到当天的日志文件
func (w *DailyRotateWriter) rotate() error {
	today := time.Now().Format("2006-01-02")
	fileName := filepath.Join(w.dir, w.prefix+"-"+today+w.ext)

	f, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %w", err)
	}

	// 关闭旧文件
	if w.file != nil {
		w.file.Close()
	}

	w.file = f
	w.current = today
	return nil
}

// Write 实现 io.Writer，每次写入检查日期是否变化
func (w *DailyRotateWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if today != w.current {
		if err := w.rotate(); err != nil {
			// 切换失败，仍然写入旧文件（降级）
			slog.Error("日志日期分割失败", "err", err)
		}
	}

	if w.file == nil {
		return 0, fmt.Errorf("日志文件未打开")
	}
	return w.file.Write(p)
}

// Close 关闭当前日志文件
func (w *DailyRotateWriter) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		w.file.Close()
		w.file = nil
	}
}

// Init 初始化日志器。
// level: "debug", "info", "warn", "error"
// jsonMode: true 输出 JSON 格式
// filePath: 日志文件路径，为空则不写文件（按日期分割）
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

// Logger 返回默认日志器
func Logger() *slog.Logger {
	if defaultLogger == nil {
		Init("info", false, "", true)
	}
	return defaultLogger
}