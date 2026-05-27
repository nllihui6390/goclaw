package store

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DialogEntry 对话条目
type DialogEntry struct {
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// DialogWriter 对话写入器（按日 JSONL 文件）
type DialogWriter struct {
	baseDir     string
	currentFile *os.File
	currentDate string
	writer      *bufio.Writer
	mu          sync.Mutex
}

// NewDialogWriter 创建对话写入器
func NewDialogWriter(baseDir string) *DialogWriter {
	return &DialogWriter{baseDir: baseDir}
}

// Append 追加对话条目
func (w *DialogWriter) Append(sessionID, role, content string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	today := time.Now().Format("2006-01-02")

	// 检查是否需要切换文件
	if w.currentDate != today || w.currentFile == nil {
		if err := w.switchFile(today); err != nil {
			return err
		}
	}

	entry := DialogEntry{
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	w.writer.Write(data)
	w.writer.WriteByte('\n')
	w.writer.Flush()

	return nil
}

// switchFile 切换到新的日期文件
func (w *DialogWriter) switchFile(date string) error {
	// 关闭旧文件
	if w.currentFile != nil {
		w.writer.Flush()
		w.currentFile.Close()
	}

	// 确保目录存在
	if err := os.MkdirAll(w.baseDir, 0755); err != nil {
		return err
	}

	// 打开新文件
	filePath := filepath.Join(w.baseDir, date+".jsonl")
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	w.currentFile = f
	w.currentDate = date
	w.writer = bufio.NewWriter(f)

	return nil
}

// Close 关闭写入器
func (w *DialogWriter) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.currentFile != nil {
		w.writer.Flush()
		w.currentFile.Close()
		w.currentFile = nil
	}
}

// ReadDialog 读取指定日期的对话
func ReadDialog(baseDir, date string) ([]DialogEntry, error) {
	filePath := filepath.Join(baseDir, date+".jsonl")
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []DialogEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry DialogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	return entries, scanner.Err()
}

// ReadRecentDialogs 读取最近 N 天的对话
func ReadRecentDialogs(baseDir string, days int) ([]DialogEntry, error) {
	var allEntries []DialogEntry

	for i := 0; i < days; i++ {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		entries, err := ReadDialog(baseDir, date)
		if err != nil {
			continue
		}
		allEntries = append(allEntries, entries...)
	}

	return allEntries, nil
}

// ListAvailableDates 列出可用的日期
func ListAvailableDates(baseDir string) ([]string, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}

	var dates []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".jsonl" {
			date := entry.Name()[:len(entry.Name())-6] // 去掉 .jsonl
			dates = append(dates, date)
		}
	}

	return dates, nil
}