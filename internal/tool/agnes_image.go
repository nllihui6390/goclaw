package tool

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go-claw/internal/channel"
	"go-claw/internal/media"
)

// AgnesImageTool 图像生成工具（Agnes-Image-2.0-Flash）
// 支持文生图、图生图、图像编辑、多图合成
type AgnesImageTool struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewAgnesImageTool 创建图像生成工具（从环境变量读取配置）
func NewAgnesImageTool() *AgnesImageTool {
	return &AgnesImageTool{
		apiKey:  os.Getenv("AGNES_API_KEY"),
		baseURL: "https://apihub.agnes-ai.com",
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // 图片生成可能较慢
		},
	}
}

func (t *AgnesImageTool) Name() string {
	return "agnes_image"
}

func (t *AgnesImageTool) Description() string {
	return `AI图像生成工具（Agnes-Image-2.0-Flash），支持以下功能：
1. 文生图：根据文本描述生成图像
2. 图生图：基于输入图像进行编辑、转换或增强
3. 图像编辑：修改构图、风格、对象、背景、场景和视觉细节
4. 多图合成：将多张参考图合成为一张新图像

使用场景：创意设计、营销内容、产品图、社交内容、角色合成等

需要配置环境变量：AGNES_API_KEY`
}

func (t *AgnesImageTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "描述目标图像或编辑需求的文本提示词。文生图时描述想要生成的内容；图生图/编辑时描述需要改变的内容和需要保持不变的内容",
			},
			"images": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
				"description": "输入图片URL/Base64/本地路径数组（可选）。支持公网URL、Data URI Base64、或本地文件路径（如 D:/path/to/image.png）。文生图时不需要此参数",
			},
			"size": map[string]interface{}{
				"type":        "string",
				"description": "输出图像尺寸，格式为 '宽x高'，如 '1024x768'、'1024x1024'、'768x1024'。默认为 '1024x1024'",
			},
		},
		"required": []string{"prompt"},
	}
}

// ImageResult 图像生成结果（标准JSON格式）
type ImageResult struct {
	Success bool     `json:"success"`
	URLs    []string `json:"urls,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// AgnesImageResult 导出给外部包使用的结果结构
type AgnesImageResult = ImageResult

// Execute 执行图像生成，调用 Agnes API 并返回 JSON 结果
func (t *AgnesImageTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.apiKey == "" {
		return "", fmt.Errorf("未配置 AGNES_API_KEY 环境变量")
	}

	// 提取参数
	prompt, ok := params["prompt"].(string)
	if !ok || prompt == "" {
		return "", fmt.Errorf("缺少 prompt 参数")
	}

	var images []string
	if imgs, ok := params["images"].([]interface{}); ok {
		for _, img := range imgs {
			if url, ok := img.(string); ok && url != "" {
				images = append(images, url)
			}
		}
	}

	size := "1024x1024"
	if s, ok := params["size"].(string); ok && s != "" {
		size = s
	}

	// 构建请求
	reqBody := map[string]interface{}{
		"model":  "agnes-image-2.0-flash",
		"prompt": prompt,
		"size":   size,
	}

	// 将 images 中的本地路径转换为 Data URI Base64
	resolvedImages := make([]string, 0, len(images))
	for _, img := range images {
		resolved := resolveImagePath(img)
		resolvedImages = append(resolvedImages, resolved)
	}
	images = resolvedImages

	if len(images) > 0 {
		reqBody["extra_body"] = map[string]interface{}{
			"image":           images,
			"response_format": "url",
		}
	} else {
		reqBody["extra_body"] = map[string]interface{}{
			"response_format": "url",
		}
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("请求序列化失败: %w", err)
	}

	url := t.baseURL + "/v1/images/generations"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqJSON))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API返回错误 (status %d): %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var result struct {
		Data []struct {
			URL *string `json:"url"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if len(result.Data) == 0 {
		return "", fmt.Errorf("未生成任何图片")
	}

	// 收集图片URL
	var urls []string
	for _, item := range result.Data {
		if item.URL != nil && *item.URL != "" {
			urls = append(urls, *item.URL)
		}
	}

	if len(urls) == 0 {
		return "", fmt.Errorf("响应中未包含图片URL")
	}

	// 返回标准JSON结果
	respResult := ImageResult{
		Success: true,
		URLs:    urls,
	}
	respJSON, _ := json.Marshal(respResult)
	return string(respJSON), nil
}

// ExecuteStructured 兼容 StructuredTool 接口（已废弃，统一走 Execute）
func (t *AgnesImageTool) ExecuteStructured(ctx context.Context, params map[string]interface{}) (channel.ContentBlocks, error) {
	result, err := t.Execute(ctx, params)
	if err != nil {
		return nil, err
	}

	var ir ImageResult
	if err := json.Unmarshal([]byte(result), &ir); err != nil || !ir.Success {
		return nil, fmt.Errorf("图像生成失败: %s", ir.Error)
	}

	var blocks channel.ContentBlocks
	for _, u := range ir.URLs {
		blocks = append(blocks, channel.NewImageBlockURL(u))
	}
	return blocks, nil
}

// resolveImagePath 将图片路径解析为 API 可用的格式
// - 本地文件路径 → Data URI Base64
// - Data URI → 原样返回
// - 公网 URL → 原样返回
func resolveImagePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}

	// 已经是 Data URI，原样返回
	if strings.HasPrefix(path, "data:image/") {
		return path
	}

	// 是 HTTP(S) URL，原样返回
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}

	// 尝试作为本地文件路径解析
	localPath := path
	// 处理 Windows 绝对路径（如 D:/xxx 或 D:\xxx）
	if matched, _ := regexp.MatchString(`^[A-Za-z]:[/\\]`, path); matched {
		localPath = path
	} else if filepath.IsAbs(path) {
		localPath = path
	} else {
		// 相对路径，相对于 clawdata 目录查找
		candidates := []string{
			filepath.Join(media.DefaultMediaDir, path),
			filepath.Join(".", path),
			path,
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				localPath = c
				break
			}
		}
	}

	// 验证文件是否存在
	info, err := os.Stat(localPath)
	if err != nil || info.IsDir() {
		// 无法找到文件，原样返回让 API 处理
		return path
	}

	// 验证是否为图片文件
	ext := strings.ToLower(filepath.Ext(localPath))
	validExts := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
		".webp": true, ".bmp": true, ".svg": true,
	}
	if !validExts[ext] {
		return path
	}

	// 读取文件并转为 Data URI
	data, err := os.ReadFile(localPath)
	if err != nil {
		return path
	}

	mimeType := media.GetMediaType(localPath)
	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)
}

func init() {
	GlobalRegistry.Register("agnes_image", func() Tool {
		return NewAgnesImageTool()
	})
}
