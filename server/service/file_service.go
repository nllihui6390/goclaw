package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileInfo 文件信息
type FileInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// FileService Agent 文件管理服务
type FileService struct {
	mu sync.RWMutex
}

// NewFileService 创建文件服务
func NewFileService() *FileService {
	return &FileService{}
}

// getAgentDir 获取 Agent 工作空间目录
func (s *FileService) getAgentDir(agentName string) string {
	return filepath.Join("clawdata", "workspaces", agentName)
}

// List 列出 Agent 工作空间的 .md 文件
func (s *FileService) List(agentName string) []FileInfo {
	agentDir := s.getAgentDir(agentName)
	files := []FileInfo{}

	entries, err := os.ReadDir(agentDir)
	if err != nil {
		return files
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		files = append(files, FileInfo{
			Name: e.Name(),
			Size: info.Size(),
		})
	}

	return files
}

// ListJSON 列出文件 JSON 字符串
func (s *FileService) ListJSON(agentName string) string {
	files := s.List(agentName)
	data, _ := json.Marshal(files)
	return string(data)
}

// Read 读取文件内容
func (s *FileService) Read(agentName, fileName string) (string, error) {
	filePath := filepath.Join(s.getAgentDir(agentName), fileName)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Write 写入文件内容
func (s *FileService) Write(agentName, fileName, content string) error {
	agentDir := s.getAgentDir(agentName)
	os.MkdirAll(agentDir, 0755)

	filePath := filepath.Join(agentDir, fileName)
	return os.WriteFile(filePath, []byte(content), 0644)
}

// Delete 删除文件
func (s *FileService) Delete(agentName, fileName string) error {
	filePath := filepath.Join(s.getAgentDir(agentName), fileName)
	return os.Remove(filePath)
}