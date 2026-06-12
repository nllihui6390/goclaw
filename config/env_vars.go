package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// EnvVarEntry 单个环境变量条目
type EnvVarEntry struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
}

// EnvVarsFile 环境变量配置文件结构（带包装）
type EnvVarsFile struct {
	Variables []EnvVarEntry `json:"variables"`
	UpdatedAt string        `json:"updated_at"`
}

// ProtectedKeys 不允许通过界面修改的危险系统环境变量
var ProtectedKeys = []string{
	"PATH", "HOME", "USER", "SHELL", "PWD", "TEMP", "TMP",
	"GOPATH", "GOROOT", "GOLANG", "LANG", "LC_ALL",
	"HOSTNAME", "PROCESSOR_ARCHITECTURE", "NUMBER_OF_PROCESSORS",
	"COMSPEC", "COMPUTERNAME", "OS",
}

// IsProtectedKey 检查 key 是否为受保护的系统变量
func IsProtectedKey(key string) bool {
	for _, pk := range ProtectedKeys {
		if strings.EqualFold(key, pk) {
			return true
		}
	}
	return false
}

// IsValidEnvVarKey 检查 key 格式是否合法（大写字母、数字、下划线）
func IsValidEnvVarKey(key string) bool {
	if key == "" {
		return false
	}
	// 首字符必须是字母或下划线
	if !((key[0] >= 'A' && key[0] <= 'Z') || key[0] == '_') {
		return false
	}
	// 其余字符必须是字母、数字或下划线
	for _, c := range key {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// GetEnvVarFilePath 获取环境变量配置文件路径（考虑 data_dir 配置）
func GetEnvVarFilePath(dataDir string) string {
	if dataDir == "" {
		dataDir = "clawdata"
	}
	return filepath.Join(dataDir, "env_vars.json")
}

// LoadAndApply 加载 env_vars.json 并将启用的变量应用到进程环境
func LoadAndApply(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在是正常的，不需要创建空文件
			return nil
		}
		return fmt.Errorf("读取环境变量配置文件失败: %w", err)
	}

	var file EnvVarsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("解析环境变量配置文件失败: %w", err)
	}

	for _, entry := range file.Variables {
		if entry.Enabled && entry.Key != "" {
			os.Setenv(entry.Key, entry.Value)
		}
	}

	return nil
}

// LoadEnvVarsFile 从文件加载环境变量配置
func LoadEnvVarsFile(filePath string) (*EnvVarsFile, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 返回空结构
			return &EnvVarsFile{Variables: []EnvVarEntry{}, UpdatedAt: ""}, nil
		}
		return nil, fmt.Errorf("读取环境变量配置文件失败: %w", err)
	}

	var file EnvVarsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("解析环境变量配置文件失败: %w", err)
	}

	return &file, nil
}

// SaveEnvVarsFile 保存环境变量配置到文件
func SaveEnvVarsFile(filePath string, file *EnvVarsFile) error {
	file.UpdatedAt = time.Now().Format(time.RFC3339)

	// 确保 data 目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化环境变量配置失败: %w", err)
	}

	// 使用 0600 权限，因为文件中可能包含密钥等敏感信息
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("写入环境变量配置文件失败: %w", err)
	}

	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// .env 来源追踪（用于 ResolveValue 区分 dotenv vs system）
// ──────────────────────────────────────────────────────────────────────────────

var (
	dotenvKeysMu    sync.RWMutex
	dotenvKeys      = make(map[string]bool)
	appliedKeysMu   sync.RWMutex
	appliedKeys     = make(map[string]bool) // 由 env_vars.json 设置的 key
)

// ComputeEnvDiff 计算环境变量差异，返回新增的 key 列表
func ComputeEnvDiff(preEnv, postEnv []string) []string {
	preMap := make(map[string]bool)
	for _, e := range preEnv {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) > 0 {
			preMap[parts[0]] = true
		}
	}

	var newKeys []string
	for _, e := range postEnv {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) > 0 && !preMap[parts[0]] {
			newKeys = append(newKeys, parts[0])
		}
	}

	return newKeys
}

// RecordDotenvKeysGlobal 记录 .env 文件加载后新增的 key（全局，由 wizard.go 调用）
func RecordDotenvKeysGlobal(keys []string) {
	dotenvKeysMu.Lock()
	for _, k := range keys {
		dotenvKeys[k] = true
	}
	dotenvKeysMu.Unlock()
}

// IsDotenvKeyGlobal 检查 key 是否来自 .env 文件
func IsDotenvKeyGlobal(key string) bool {
	dotenvKeysMu.RLock()
	defer dotenvKeysMu.RUnlock()
	return dotenvKeys[key]
}

// IsAppliedKeyGlobal 检查 key 是否由 env_vars.json 设置
func IsAppliedKeyGlobal(key string) bool {
	appliedKeysMu.RLock()
	defer appliedKeysMu.RUnlock()
	return appliedKeys[key]
}

// RecordAppliedKey 记录由 env_vars.json 设置的 key
func RecordAppliedKey(key string) {
	appliedKeysMu.Lock()
	appliedKeys[key] = true
	appliedKeysMu.Unlock()
}

// ClearAppliedKey 清除由 env_vars.json 设置的 key 记录
func ClearAppliedKey(key string) {
	appliedKeysMu.Lock()
	delete(appliedKeys, key)
	appliedKeysMu.Unlock()
}

// GetAppliedKeys 获取所有由 env_vars.json 设置的 key
func GetAppliedKeys() map[string]bool {
	appliedKeysMu.RLock()
	defer appliedKeysMu.RUnlock()
	result := make(map[string]bool)
	for k, v := range appliedKeys {
		result[k] = v
	}
	return result
}

// SetAppliedKeys 设置所有由 env_vars.json 设置的 key（用于批量更新）
func SetAppliedKeys(keys map[string]bool) {
	appliedKeysMu.Lock()
	appliedKeys = keys
	appliedKeysMu.Unlock()
}