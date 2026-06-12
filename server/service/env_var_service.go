package service

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"go-claw/config"
	"go-claw/global"
	glog "go-claw/pkg/log"
)

// EnvVarService 环境变量管理服务
type EnvVarService struct {
	mu       sync.RWMutex
	dataFile string
	config   *ConfigService
}

// NewEnvVarService 创建环境变量管理服务
func NewEnvVarService(config *ConfigService) *EnvVarService {
	dataDir := "clawdata"
	gateway, _ := config.Get()["gateway"].(map[string]interface{})
	if gateway != nil {
		if v, ok := gateway["data_dir"].(string); ok && v != "" {
			dataDir = v
		}
	}

	s := &EnvVarService{
		dataFile: config.GetEnvVarFilePath(dataDir),
		config:   config,
	}

	// 启动时应用一次已保存的环境变量
	s.ApplyToProcess()

	return s
}

// GetEnvVarFilePath 获取环境变量配置文件路径（考虑 data_dir 配置）
func (cs *ConfigService) GetEnvVarFilePath(dataDir string) string {
	return config.GetEnvVarFilePath(dataDir)
}

// List 获取所有环境变量
func (s *EnvVarService) List() []config.EnvVarEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	file, err := config.LoadEnvVarsFile(s.dataFile)
	if err != nil {
		return nil
	}
	return file.Variables
}

// ListJSON 获取所有环境变量的 JSON 字符串
func (s *EnvVarService) ListJSON() string {
	entries := s.List()
	file := &config.EnvVarsFile{
		Variables: entries,
	}
	data, _ := json.Marshal(file)
	return string(data)
}

// ListWithSource 获取所有环境变量并附加来源信息
func (s *EnvVarService) ListWithSource() []map[string]interface{} {
	entries := s.List()
	result := make([]map[string]interface{}, 0, len(entries))

	for _, entry := range entries {
		_, source := s.ResolveValue(entry.Key)
		item := map[string]interface{}{
			"key":         entry.Key,
			"value":       entry.Value,
			"description": entry.Description,
			"enabled":     entry.Enabled,
			"source":      source,
		}
		result = append(result, item)
	}

	return result
}

// Save 添加新环境变量
func (s *EnvVarService) Save(entry config.EnvVarEntry) error {
	// 验证 key 格式
	if !config.IsValidEnvVarKey(entry.Key) {
		return fmt.Errorf("无效的环境变量名: %s (只允许大写字母、数字、下划线，首字符必须是字母或下划线)", entry.Key)
	}

	// 检查受保护的 key
	if config.IsProtectedKey(entry.Key) {
		return fmt.Errorf("不允许修改受保护的系统变量: %s", entry.Key)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := config.LoadEnvVarsFile(s.dataFile)
	if err != nil {
		return err
	}

	// 检查重复
	for _, v := range file.Variables {
		if v.Key == entry.Key {
			return fmt.Errorf("环境变量 %s 已存在", entry.Key)
		}
	}

	file.Variables = append(file.Variables, entry)

	if err := config.SaveEnvVarsFile(s.dataFile, file); err != nil {
		return err
	}

	// 自动应用
	go s.ApplyToProcess()

	return nil
}

// Update 更新环境变量
func (s *EnvVarService) Update(entry config.EnvVarEntry) error {
	// 验证 key 格式
	if !config.IsValidEnvVarKey(entry.Key) {
		return fmt.Errorf("无效的环境变量名: %s", entry.Key)
	}

	// 检查受保护的 key
	if config.IsProtectedKey(entry.Key) {
		return fmt.Errorf("不允许修改受保护的系统变量: %s", entry.Key)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := config.LoadEnvVarsFile(s.dataFile)
	if err != nil {
		return err
	}

	found := false
	for i, v := range file.Variables {
		if v.Key == entry.Key {
			file.Variables[i] = entry
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("环境变量 %s 不存在", entry.Key)
	}

	if err := config.SaveEnvVarsFile(s.dataFile, file); err != nil {
		return err
	}

	// 自动应用
	go s.ApplyToProcess()

	return nil
}

// Delete 删除环境变量
func (s *EnvVarService) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := config.LoadEnvVarsFile(s.dataFile)
	if err != nil {
		return err
	}

	found := false
	filtered := make([]config.EnvVarEntry, 0, len(file.Variables))
	for _, v := range file.Variables {
		if v.Key == key {
			found = true
			continue
		}
		filtered = append(filtered, v)
	}

	if !found {
		return fmt.Errorf("环境变量 %s 不存在", key)
	}

	file.Variables = filtered

	if err := config.SaveEnvVarsFile(s.dataFile, file); err != nil {
		return err
	}

	// 清除进程中的该变量（仅清除由 config 文件设置的）
	if config.IsAppliedKeyGlobal(key) {
		os.Unsetenv(key)
		config.ClearAppliedKey(key)
	}

	// 自动重载配置
	go func() {
		global.ReloadConfigAndSyncAgents()
		glog.Logger().Info("[EnvVar] 删除环境变量后重载配置", "key", key)
	}()

	return nil
}

// ApplyToProcess 将所有启用的环境变量应用到进程
func (s *EnvVarService) ApplyToProcess() error {
	s.mu.RLock()
	entries := s.List()
	s.mu.RUnlock()

	newApplied := make(map[string]bool)
	for _, entry := range entries {
		if entry.Enabled {
			os.Setenv(entry.Key, entry.Value)
			newApplied[entry.Key] = true
		}
	}
	// 清除之前由 config 文件设置但现在已不存在或被禁用的变量
	oldApplied := config.GetAppliedKeys()
	for key := range oldApplied {
		if !newApplied[key] {
			os.Unsetenv(key)
		}
	}
	config.SetAppliedKeys(newApplied)

	// 触发配置重载，使 Agent 重新读取 provider 配置（os.Getenv 覆盖）
	global.ReloadConfigAndSyncAgents()

	glog.Logger().Info("[EnvVar] 环境变量已应用到进程", "count", len(newApplied))

	return nil
}

// ResolveValue 按优先级解析环境变量的值
// 优先级: env_vars.json (config) > .env (dotenv) > 系统环境变量 (system)
func (s *EnvVarService) ResolveValue(key string) (string, string) {
	// 1. 检查 env_vars.json（最高优先级）
	s.mu.RLock()
	entries := s.List()
	s.mu.RUnlock()

	for _, entry := range entries {
		if entry.Key == key && entry.Enabled {
			return entry.Value, "config"
		}
	}

	// 2. 检查 os.Getenv()（包含 .env 和系统环境变量）
	// 注意: godotenv.Overload() 已将 .env 值加载到进程环境
	// 由于 godotenv.Overload() 使 .env 覆盖系统变量，进程环境中的值就是 .env 或系统值
	val := os.Getenv(key)
	if val != "" {
		// 尝试区分 .env 和系统来源
		if config.IsDotenvKeyGlobal(key) {
			return val, "dotenv"
		}
		return val, "system"
	}

	return "", "none"
}

// ReloadEnvVarsFile 强制重新加载环境变量文件并应用到进程
func (s *EnvVarService) ReloadEnvVarsFile() error {
	s.mu.Lock()
	s.mu.Unlock()

	// 重新读取并应用
	return s.ApplyToProcess()
}