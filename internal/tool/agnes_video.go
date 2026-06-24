package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"go-claw/internal/channel"
	"go-claw/internal/media"
)

// AgnesVideoTool 视频生成工具（Agnes-Video-V2.0）
// 支持文生视频、图生视频、多图视频、关键帧动画
type AgnesVideoTool struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewAgnesVideoTool 创建视频生成工具
func NewAgnesVideoTool() *AgnesVideoTool {
	return &AgnesVideoTool{
		apiKey:  os.Getenv("AGNES_API_KEY"),
		baseURL: "https://apihub.agnes-ai.com",
		httpClient: &http.Client{
			Timeout: 10 * time.Minute, // 视频生成较慢
		},
	}
}

func (t *AgnesVideoTool) Name() string {
	return "agnes_video"
}

func (t *AgnesVideoTool) Description() string {
	return `AI视频生成工具（Agnes-Video-V2.0），支持以下功能：
1. 文生视频：根据文本描述直接生成视频
2. 图生视频：将静态图片动画化为动态视频
3. 多图视频生成：使用多张参考图片指导视频生成
4. 关键帧动画：在多个关键帧之间生成平滑过渡

参数说明：
- prompt: 视频描述文字（必填）
- images: 输入图片URL/本地路径数组（可选，图生视频/多图/关键帧时使用）
- mode: 生成模式，"ti2vid"（文生视频，默认）或 "keyframes"（关键帧动画）
- width: 视频宽度，默认 1152
- height: 视频高度，默认 768
- num_frames: 视频帧数，必须 ≤ 441 且满足 8n+1（如 81, 121, 241, 441），默认 121
- frame_rate: 视频帧率，范围 1-60，默认 24
- negative_prompt: 负向提示词，描述需要避免的内容（可选）

常用帧数参数：
- 约 3 秒: num_frames=81, frame_rate=24
- 约 5 秒: num_frames=121, frame_rate=24
- 约 10 秒: num_frames=241, frame_rate=24
- 约 18 秒: num_frames=441, frame_rate=24

支持本地文件路径（如 D:/path/to/image.png），自动转为 Base64 发送。

需要配置环境变量：AGNES_API_KEY`
}

func (t *AgnesVideoTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "视频描述文字，描述主体、动作、场景、镜头运动、光照和风格",
			},
			"images": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
				"description": "输入图片URL/Base64/本地路径数组（可选）。图生视频传单张图片，多图视频/关键帧动画传多张",
			},
			"mode": map[string]interface{}{
				"type":        "string",
				"description": "生成模式：ti2vid（文生视频，默认）或 keyframes（关键帧动画）",
			},
			"width": map[string]interface{}{
				"type":        "integer",
				"description": "视频宽度，默认 1152",
			},
			"height": map[string]interface{}{
				"type":        "integer",
				"description": "视频高度，默认 768",
			},
			"num_frames": map[string]interface{}{
				"type":        "integer",
				"description": "视频总帧数，必须 ≤ 441 且满足 8n+1（如 81, 121, 161, 241, 441）。默认 121",
			},
			"frame_rate": map[string]interface{}{
				"type":        "integer",
				"description": "视频帧率 FPS，范围 1-60，默认 24",
			},
			"negative_prompt": map[string]interface{}{
				"type":        "string",
				"description": "负向提示词，描述需要避免的内容",
			},
		},
		"required": []string{"prompt"},
	}
}

// VideoResult 视频生成结果
type VideoResult struct {
	Success bool     `json:"success"`
	URLs    []string `json:"urls,omitempty"`
	TaskID  string   `json:"task_id,omitempty"`
	VideoID string   `json:"video_id,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// Execute 执行视频生成，异步任务模式：创建任务 → 轮询结果
func (t *AgnesVideoTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.apiKey == "" {
		return "", fmt.Errorf("未配置 AGNES_API_KEY 环境变量")
	}

	// 提取参数
	prompt, ok := params["prompt"].(string)
	if !ok || prompt == "" {
		return "", fmt.Errorf("缺少 prompt 参数")
	}

	mode, _ := params["mode"].(string)
	if mode == "" {
		mode = "ti2vid"
	}

	width, _ := params["width"].(float64)
	height, _ := params["height"].(float64)
	numFrames, _ := params["num_frames"].(float64)
	frameRate, _ := params["frame_rate"].(float64)
	negativePrompt, _ := params["negative_prompt"].(string)

	var images []string
	if imgs, ok := params["images"].([]interface{}); ok {
		for _, img := range imgs {
			if url, ok := img.(string); ok && url != "" {
				images = append(images, url)
			}
		}
	}

	// 解析本地路径
	resolvedImages := make([]string, 0, len(images))
	for _, img := range images {
		resolved := resolveImagePath(img)
		resolvedImages = append(resolvedImages, resolved)
	}
	images = resolvedImages

	// 默认参数
	if width == 0 {
		width = 1152
	}
	if height == 0 {
		height = 768
	}
	if numFrames == 0 {
		numFrames = 121
	}
	if frameRate == 0 {
		frameRate = 24
	}

	// 构建创建任务请求
	reqBody := map[string]interface{}{
		"model":    "agnes-video-v2.0",
		"prompt":   prompt,
		"width":    int(width),
		"height":   int(height),
		"num_frames": int(numFrames),
		"frame_rate": int(frameRate),
	}
	if negativePrompt != "" {
		reqBody["negative_prompt"] = negativePrompt
	}

	// 设置图片参数
	if len(images) > 0 {
		if mode == "keyframes" {
			reqBody["extra_body"] = map[string]interface{}{
				"image": images,
				"mode":  "keyframes",
			}
		} else if len(images) == 1 {
			// 单张图片走 image 字段（图生视频）
			reqBody["image"] = images[0]
		} else {
			// 多张图片走 extra_body.image
			reqBody["extra_body"] = map[string]interface{}{
				"image": images,
			}
		}
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("请求序列化失败: %w", err)
	}

	// 创建视频任务
	taskResp, err := t.createVideoTask(ctx, reqJSON)
	if err != nil {
		return "", err
	}

	// 轮询查询结果
	videoURL, err := t.pollVideoResult(ctx, taskResp.TaskID, taskResp.VideoID)
	if err != nil {
		return "", err
	}

	// 下载视频到本地
	localPath, err := SaveGeneratedVideo(videoURL, "agnes_video")
	if err != nil {
		return fmt.Sprintf("⚠️ 视频已生成但保存失败: %v（远程 URL: %s）", err, videoURL), nil
	}

	// 返回标准JSON结果
	respResult := VideoResult{
		Success: true,
		URLs:    []string{localPath},
		TaskID:  taskResp.TaskID,
		VideoID: taskResp.VideoID,
	}
	respJSON, _ := json.Marshal(respResult)
	return string(respJSON), nil
}

// VideoTaskResponse 创建视频任务的响应
type VideoTaskResponse struct {
	TaskID  string
	VideoID string
	Status  string
}

// createVideoTask 创建视频生成任务
func (t *AgnesVideoTool) createVideoTask(ctx context.Context, reqJSON []byte) (*VideoTaskResponse, error) {
	url := t.baseURL + "/v1/videos"

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API返回错误 (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		TaskID  string `json:"task_id"`
		VideoID string `json:"video_id"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if result.TaskID == "" {
		return nil, fmt.Errorf("响应中缺少 task_id")
	}

	return &VideoTaskResponse{
		TaskID:  result.TaskID,
		VideoID: result.VideoID,
		Status:  result.Status,
	}, nil
}

// pollVideoResult 轮询查询视频生成结果
func (t *AgnesVideoTool) pollVideoResult(ctx context.Context, taskID, videoID string) (string, error) {
	const pollInterval = 5 * time.Second
	const maxAttempts = 120 // 最多轮询 10 分钟

	for i := 0; i < maxAttempts; i++ {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("上下文取消")
		default:
		}

		time.Sleep(pollInterval)

		url := t.baseURL + "/agnesapi?video_id=" + videoID
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return "", fmt.Errorf("创建查询请求失败: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+t.apiKey)

		resp, err := t.httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("查询请求失败: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("读取响应失败: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("查询失败 (status %d): %s", resp.StatusCode, string(body))
		}

		var result struct {
			Status string `json:"status"`
			Error  string `json:"error"`
			URL    string `json:"remixed_from_video_id"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return "", fmt.Errorf("解析响应失败: %w", err)
		}

		switch result.Status {
		case "completed":
			if result.URL == "" {
				return "", fmt.Errorf("视频已完成但响应中缺少 URL")
			}
			return result.URL, nil
		case "failed":
			return "", fmt.Errorf("视频生成失败: %s", result.Error)
		case "queued", "in_progress":
			// 继续轮询
			if i%6 == 0 {
				// 每 30 秒打印一次进度日志
				// 不打印日志，保持安静
			}
		default:
			return "", fmt.Errorf("未知状态: %s", result.Status)
		}
	}

	return "", fmt.Errorf("视频生成超时（超过 %d 秒）", maxAttempts*int(pollInterval.Seconds()))
}

// SaveGeneratedVideo 下载视频到本地
func SaveGeneratedVideo(videoURL, subdir string) (string, error) {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(videoURL)
	if err != nil {
		return "", fmt.Errorf("下载视频失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载视频失败: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取视频数据失败: %v", err)
	}

	return saveVideo(data, subdir)
}

func saveVideo(data []byte, subdir string) (string, error) {
	dir := filepath.Join(media.DefaultMediaDir, subdir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %v", err)
	}

	filename := fmt.Sprintf("%s_%d.mp4", subdir, time.Now().UnixNano())
	path := filepath.Join(dir, filename)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("保存视频失败: %v", err)
	}

	return path, nil
}

// ExecuteStructured 兼容 StructuredTool 接口（已废弃，统一走 Execute）
func (t *AgnesVideoTool) ExecuteStructured(ctx context.Context, params map[string]interface{}) (channel.ContentBlocks, error) {
	result, err := t.Execute(ctx, params)
	if err != nil {
		return nil, err
	}

	var vr VideoResult
	if err := json.Unmarshal([]byte(result), &vr); err != nil || !vr.Success {
		return nil, fmt.Errorf("视频生成失败: %s", vr.Error)
	}

	var blocks channel.ContentBlocks
	for _, u := range vr.URLs {
		blocks = append(blocks, channel.NewVideoBlockURL(u))
	}
	return blocks, nil
}

func init() {
	GlobalRegistry.Register("agnes_video", func() Tool {
		return NewAgnesVideoTool()
	})
}
