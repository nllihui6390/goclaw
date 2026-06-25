package utils

import "strings"

// SensitiveKeywords 敏感关键词列表（用于路径安全检查）
var SensitiveKeywords = []string{
	".env",
	".git",
	"credentials",
	"secret",
	"password",
	"private_key",
	"id_rsa",
	"authorized_keys",
	"shadow",
	"passwd",
}

// DownloadSensitiveKeywords 下载额外的敏感关键词
var DownloadSensitiveKeywords = append(SensitiveKeywords, ".ssh", ".gnupg")

// IsSensitivePath 检查路径是否包含敏感关键词
func IsSensitivePath(path string) bool {
	lower := strings.ToLower(path)
	for _, s := range SensitiveKeywords {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// IsSensitiveDownloadPath 检查下载路径是否敏感（更严格的版本）
func IsSensitiveDownloadPath(path string) bool {
	lower := strings.ToLower(path)
	for _, s := range DownloadSensitiveKeywords {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}
