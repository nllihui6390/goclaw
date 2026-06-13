package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// SummaryData 会话摘要持久化数据（独立文件存储，不影响 SessionData）
type SummaryData struct {
	Summary         string `json:"summary"`          // 结构化摘要文本
	CompressedCount int    `json:"compressed_count"` // 已被压缩的消息数量
	UpdatedAt       string `json:"updated_at"`       // 更新时间
}

// SummaryFilePath 根据 session 文件路径计算摘要文件路径
// sessionPath 如：<dataDir>/sessions/<id>.json
// summaryPath 如：<dataDir>/sessions/<id>_summary.json
func SummaryFilePath(sessionPath string) string {
	ext := filepath.Ext(sessionPath)
	return sessionPath[:len(sessionPath)-len(ext)] + "_summary.json"
}

// LoadSummary 从文件加载摘要
func LoadSummary(sessionPath string) (*SummaryData, error) {
	summaryPath := SummaryFilePath(sessionPath)
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 文件不存在是正常情况
		}
		return nil, err
	}

	var sd SummaryData
	if err := json.Unmarshal(data, &sd); err != nil {
		return nil, err
	}
	return &sd, nil
}

// SaveSummary 保存摘要到文件
func SaveSummary(sessionPath string, sd *SummaryData) error {
	if sd == nil {
		return nil
	}
	sd.UpdatedAt = time.Now().Format(time.RFC3339)

	summaryPath := SummaryFilePath(sessionPath)
	data, err := json.MarshalIndent(sd, "", "  ")
	if err != nil {
		return err
	}

	// 确保目录存在
	dir := filepath.Dir(summaryPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(summaryPath, data, 0644)
}

// DeleteSummary 删除摘要文件
func DeleteSummary(sessionPath string) error {
	summaryPath := SummaryFilePath(sessionPath)
	return os.Remove(summaryPath)
}
