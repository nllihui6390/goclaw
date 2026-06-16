package token_usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	glog "go-claw/pkg/log"
)

// Storage Token 使用量持久化存储
type Storage struct {
	path string
	mu   sync.Mutex
}

// NewStorage 创建存储实例
func NewStorage(path string) *Storage {
	return &Storage{path: path}
}

// Load 从文件加载 Token 使用量数据
func (s *Storage) Load() map[string]map[string]UsageEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			glog.Logger().Warn("token_usage: 读取文件失败", "path", s.path, "err", err)
		}
		glog.Logger().Info("token_usage: 文件不存在或读取失败", "path", s.path)
		return make(map[string]map[string]UsageEntry)
	}

	if len(data) == 0 {
		glog.Logger().Info("token_usage: 文件为空", "path", s.path)
		return make(map[string]map[string]UsageEntry)
	}

	var result map[string]map[string]UsageEntry
	if err := json.Unmarshal(data, &result); err != nil {
		glog.Logger().Warn("token_usage: 解析 JSON 失败", "path", s.path, "err", err)
		return make(map[string]map[string]UsageEntry)
	}

	dayCount := len(result)
	totalEntries := 0
	for _, dayData := range result {
		totalEntries += len(dayData)
	}
	glog.Logger().Info("token_usage: 文件加载成功", "path", s.path, "days", dayCount, "entries", totalEntries)
	return result
}

// Save 保存 Token 使用量数据到文件（原子写入）
func (s *Storage) Save(data map[string]map[string]UsageEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 确保目录存在
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		glog.Logger().Warn("token_usage: 创建目录失败", "dir", dir, "err", err)
		return
	}

	// 原子写入：先写临时文件，再替换
	tmpPath := s.path + ".tmp"
	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		glog.Logger().Warn("token_usage: 序列化 JSON 失败", "err", err)
		return
	}

	if err := os.WriteFile(tmpPath, payload, 0644); err != nil {
		glog.Logger().Warn("token_usage: 写入临时文件失败", "path", tmpPath, "err", err)
		return
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		glog.Logger().Warn("token_usage: 替换文件失败", "err", err)
		// 清理临时文件
		os.Remove(tmpPath)
		return
	}

	glog.Logger().Debug("token_usage: 数据已保存到磁盘")
}