package service

import (
	"encoding/json"
)

// ToolInfo 工具信息
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	SkillGroup  string `json:"skill_group"`
}

// toolDescriptions 工具描述映射
var toolDescriptions = map[string]string{
	"weather":            "天气查询工具",
	"exec":               "执行Shell命令",
	"write_file":         "写入文件",
	"read_file":          "读取文件",
	"edit_file":          "编辑文件",
	"append_file":        "追加文件内容",
	"send_file":          "发送文件",
	"browser_use":        "浏览器自动化",
	"get_current_time":   "获取当前时间",
	"set_user_timezone":  "设置时区",
	"cron_status":        "定时任务状态",
	"system_info":        "系统信息",
	"network_check":      "网络检测",
	"http_request":       "HTTP请求",
	"web_search":         "网络搜索",
	"calculate":          "计算器",
	"run_code":           "运行代码",
	"list_files":         "列出文件",
	"read_pdf":           "读取PDF",
	"ocr_image":          "OCR图像识别",
	"generate_image":     "生成图像",
	"database_query":     "数据库查询",
	"manage_config":      "管理配置",
}

// ToolService 工具管理服务
type ToolService struct {
	config *ConfigService
}

// NewToolService 创建工具服务
func NewToolService(config *ConfigService) *ToolService {
	return &ToolService{config: config}
}

// List 获取工具列表
func (s *ToolService) List() []ToolInfo {
	agents := s.config.GetAgents()
	toolsMap := map[string]bool{}

	for _, ag := range agents {
		ts, _ := ag["tools"].([]interface{})
		for _, t := range ts {
			toolName, _ := t.(string)
			toolsMap[toolName] = true
		}
	}

	tools := []ToolInfo{}
	for name := range toolsMap {
		desc := toolDescriptions[name]
		if desc == "" {
			desc = "自定义工具"
		}
		tools = append(tools, ToolInfo{
			Name:        name,
			Description: desc,
			SkillGroup:  "builtin",
		})
	}

	return tools
}

// ListJSON 获取工具列表 JSON 字符串
func (s *ToolService) ListJSON() string {
	tools := s.List()
	data, _ := json.Marshal(tools)
	return string(data)
}

// ListSimple 获取简单工具名称列表
func (s *ToolService) ListSimple() []map[string]string {
	tools := s.List()
	result := make([]map[string]string, 0, len(tools))
	for _, t := range tools {
		result = append(result, map[string]string{"name": t.Name})
	}
	return result
}

// ListSimpleJSON 获取简单工具名称列表 JSON 字符串
func (s *ToolService) ListSimpleJSON() string {
	result := s.ListSimple()
	data, _ := json.Marshal(result)
	return string(data)
}