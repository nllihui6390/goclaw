package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	
	"go-claw/internal/media"
)

// GenerateImageTool AI图片生成工具
type GenerateImageTool struct{}

func NewGenerateImageTool() *GenerateImageTool {
	return &GenerateImageTool{}
}

func (t *GenerateImageTool) Name() string {
	return "generate_image"
}

func (t *GenerateImageTool) Description() string {
	return `使用 AI 生成图片。
根据文字描述生成对应的图片，支持多种风格。

调用格式：
- generate_image(prompt="一只可爱的猫咪坐在窗台上")  # 生成图片
- generate_image(prompt="山水画风格的城市夜景", style="chinese_painting")  # 指定风格
- generate_image(prompt="sunset over mountains", size="512x512")  # 指定尺寸

参数说明：
- prompt: 图片描述文字（必填）
- style: 图片风格（可选）
- size: 图片尺寸，默认 512x512（可选）

支持的风格：
- realistic: 写实风格
- anime: 动漫风格
- chinese_painting: 国画风格
- watercolor: 水彩风格
- oil_painting: 油画风格
- pixel_art: 像素风格

支持的后端：
- OpenAI DALL-E（需 OPENAI_API_KEY）
- SiliconFlow（需 SILICONFLOW_API_KEY）
- ZhipuAI CogView（需 ZHIPU_API_KEY）`
}

func (t *GenerateImageTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "图片描述文字",
			},
			"style": map[string]interface{}{
				"type":        "string",
				"description": "图片风格: realistic, anime, chinese_painting, watercolor, oil_painting, pixel_art",
			},
			"size": map[string]interface{}{
				"type":        "string",
				"description": "图片尺寸，默认 512x512",
			},
			"save_path": map[string]interface{}{
				"type":        "string",
				"description": "保存到本地路径（可选，默认不保存）",
			},
		},
		"required": []string{"prompt"},
	}
}

// Execute 执行图片生成（只返回文本结果给 LLM）
func (t *GenerateImageTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	prompt, ok := params["prompt"].(string)
	if !ok || prompt == "" {
		return "", fmt.Errorf("缺少 prompt 参数")
	}

	style, _ := params["style"].(string)
	size, _ := params["size"].(string)
	savePath, _ := params["save_path"].(string)

	if size == "" {
		size = "512x512"
	}

	// 根据风格增强 prompt
	if style != "" {
		prompt = enhancePrompt(prompt, style)
	}

	// 尝试不同的后端
	// 1. SiliconFlow
	sfKey := getEnv("SILICONFLOW_API_KEY", "")
	if sfKey != "" {
		return t.generateSiliconFlow(prompt, size, sfKey, savePath)
	}

	// 2. ZhipuAI CogView
	zpKey := getEnv("ZHIPU_API_KEY", "")
	if zpKey != "" {
		return t.generateCogView(prompt, size, zpKey, savePath)
	}

	// 3. OpenAI DALL-E
	oaiKey := getEnv("OPENAI_API_KEY", "")
	if oaiKey != "" {
		return t.generateDallE(prompt, size, oaiKey, savePath)
	}

	return "", fmt.Errorf("未配置图片生成 API Key，请设置以下环境变量之一：\n- SILICONFLOW_API_KEY\n- ZHIPU_API_KEY\n- OPENAI_API_KEY")
}

func (t *GenerateImageTool) generateDallE(prompt, size, apiKey, savePath string) (string, error) {
	baseURL := getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1")
	url := baseURL + "/images/generations"

	body := map[string]interface{}{
		"model":  "dall-e-3",
		"prompt": prompt,
		"n":      1,
		"size":   size,
	}
	bodyJSON, _ := json.Marshal(body)

	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API 返回错误 (%d): %s", resp.StatusCode, string(respBody))
	}

	var result dallEResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	if len(result.Data) == 0 {
		return "", fmt.Errorf("未生成图片")
	}

	imageURL := result.Data[0].URL
	return t.buildImageResult(imageURL, "dalle", savePath)
}

func (t *GenerateImageTool) generateSiliconFlow(prompt, size, apiKey, savePath string) (string, error) {
	url := "https://api.siliconflow.cn/v1/images/generations"

	body := map[string]interface{}{
		"model":  "Kwai-Kolors/Kolors",
		"prompt": prompt,
		"n":      1,
		"size":   size,
	}
	bodyJSON, _ := json.Marshal(body)

	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API 返回错误 (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	if len(result.Data) == 0 {
		return "", fmt.Errorf("未生成图片")
	}

	imageURL := result.Data[0].URL
	return t.buildImageResult(imageURL, "siliconflow", savePath)
}

func (t *GenerateImageTool) generateCogView(prompt, size, apiKey, savePath string) (string, error) {
	url := "https://open.bigmodel.cn/api/paas/v4/images/generations"

	body := map[string]interface{}{
		"model":  "cogview-3",
		"prompt": prompt,
		"size":   size,
	}
	bodyJSON, _ := json.Marshal(body)

	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API 返回错误 (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	if len(result.Data) == 0 {
		return "", fmt.Errorf("未生成图片")
	}

	imageURL := result.Data[0].URL
	return t.buildImageResult(imageURL, "cogview", savePath)
}

// buildImageResult 构建图片生成结果（只返回文本给 LLM）
// 优先保存到本地；如果保存失败则使用远程 URL
func (t *GenerateImageTool) buildImageResult(imageURL, subdir, savePath string) (string, error) {
	// 优先保存到本地媒体目录
	localPath, err := media.SaveGeneratedImageFromURL(imageURL, subdir)
	if err == nil {
		return fmt.Sprintf("✅ 图片已生成并保存到: %s", localPath), nil
	}

	// 保存失败但指定了 save_path
	if savePath != "" {
		if err := downloadAndSave(imageURL, savePath); err != nil {
			return fmt.Sprintf("⚠️ 图片已生成但保存失败: %v（远程 URL: %s）", err, imageURL), nil
		}
		return fmt.Sprintf("✅ 图片已生成并保存到: %s", savePath), nil
	}

	// 全部失败：直接使用远程 URL
	return fmt.Sprintf("✅ 图片已生成（远程 URL: %s）", imageURL), nil
}

func enhancePrompt(prompt, style string) string {
	styleMap := map[string]string{
		"realistic":         "photorealistic, high detail, professional photography",
		"anime":             "anime style, Japanese animation art, vibrant colors",
		"chinese_painting":  "traditional Chinese ink painting, elegant, minimalist",
		"watercolor":        "watercolor painting, soft colors, artistic",
		"oil_painting":      "oil painting, rich textures, classical art",
		"pixel_art":         "pixel art style, retro game aesthetic",
	}
	if suffix, ok := styleMap[style]; ok {
		return prompt + ", " + suffix
	}
	return prompt
}

func downloadAndSave(url, savePath string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("下载图片失败: %v", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取图片数据失败: %v", err)
	}

	return os.WriteFile(savePath, data, 0644)
}

type dallEResponse struct {
	Data []struct {
		URL string `json:"url"`
	} `json:"data"`
}

func init() {
	GlobalRegistry.Register("generate_image", func() Tool {
		return NewGenerateImageTool()
	})
}