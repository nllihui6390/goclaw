package bootstrap

import (
	"go-claw/config"
)

// WriteInitialDefaults 写入首次运行的默认配置并返回根配置
func WriteInitialDefaults() *config.Config {
	return config.GetDefaultRootConfig()
}

// WriteInitialConfigs 写入初始 Agent 配置文件（首次启动时调用）
func WriteInitialConfigs(workspaceDir string) error {
	return config.WriteInitialConfigs(workspaceDir)
}

// GetDefaultRootConfig 返回默认根配置（统一入口，复用 config 包）
func GetDefaultRootConfig() *config.Config {
	return config.GetDefaultRootConfig()
}

