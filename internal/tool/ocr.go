package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// OCRImageTool 图片文字识别工具
type OCRImageTool struct{}

func NewOCRImageTool() *OCRImageTool {
	return &OCRImageTool{}
}

func (t *OCRImageTool) Name() string {
	return "ocr_image"
}

func (t *OCRImageTool) Description() string {
	return `识别图片中的文字内容（OCR）。
支持本地图片文件路径或图片 URL。
可识别中英文混合文本、表格、代码等。

调用格式：
- ocr_image(path="screenshot.png")  # 识别本地图片
- ocr_image(path="https://example.com/image.jpg")  # 识别网络图片
- ocr_image(path="photo.png", language="ch_sim")  # 指定语言

参数说明：
- path: 图片文件路径或 URL（必填）
- language: 识别语言，默认 ch_sim（简体中文+英文）

支持的语言：
- ch_sim: 简体中文+英文
- ch_tra: 繁体中文+英文
- eng: 纯英文
- jpn: 日文
- kor: 韩文

依赖：需要系统安装 Tesseract OCR 或 Python Pillow+pytesseract`
}

func (t *OCRImageTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "图片文件路径或 URL",
			},
			"language": map[string]interface{}{
				"type":        "string",
				"description": "识别语言，默认 ch_sim（简体中文+英文）",
			},
		},
		"required": []string{"path"},
	}
}

// OCRResult OCR识别结果JSON结构
type OCRResult struct {
	Source     string  `json:"source"`
	Language   string  `json:"language"`
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	Truncated  bool    `json:"truncated,omitempty"`
}

func (t *OCRImageTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("缺少 path 参数")
	}

	language := "ch_sim"
	if lang, ok := params["language"].(string); ok && lang != "" {
		language = lang
	}

	// 获取图片数据
	imageData, err := getImageData(path)
	if err != nil {
		return "", err
	}

	// 方式1：使用 Tesseract CLI
	if _, err := exec.LookPath("tesseract"); err == nil {
		return t.ocrWithTesseract(imageData, language, path)
	}

	// 方式2：使用 Python pytesseract
	if hasPyModule("pytesseract") {
		return t.ocrWithPython(imageData, language, path)
	}

	return "", fmt.Errorf("未找到 OCR 工具，请安装 Tesseract OCR\nWindows: https://github.com/UB-Mannheim/tesseract/wiki\nMac: brew install tesseract\nLinux: apt install tesseract-ocr tesseract-ocr-chi-sim")
}

func getImageData(path string) ([]byte, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		client := &http.Client{Timeout: 20 * time.Second}
		resp, err := client.Get(path)
		if err != nil {
			return nil, fmt.Errorf("下载图片失败: %v", err)
		}
		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}

	if isSensitivePath(path) {
		return nil, fmt.Errorf("禁止读取敏感路径: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取图片文件失败: %v", err)
	}
	return data, nil
}

func (t *OCRImageTool) ocrWithTesseract(imageData []byte, language, source string) (string, error) {
	// 保存临时图片文件（在 clawdata/tmp 目录）
	tmpImg := getTmpFile("goclaw_ocr_"+fmt.Sprintf("%d", time.Now().UnixNano()), ".png")
	if err := os.WriteFile(tmpImg, imageData, 0644); err != nil {
		return "", fmt.Errorf("保存临时图片失败: %v", err)
	}
	defer os.Remove(tmpImg)

	// 保存临时输出文件
	tmpOut := getTmpFile("goclaw_ocr_"+fmt.Sprintf("%d", time.Now().UnixNano()), "")
	defer os.Remove(tmpOut + ".txt")

	cmd := exec.Command("tesseract", tmpImg, tmpOut, "-l", language, "--psm", "6")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("Tesseract 识别失败: %v\n%s", err, stderr.String())
	}

	content, err := os.ReadFile(tmpOut + ".txt")
	if err != nil {
		return "", fmt.Errorf("读取识别结果失败: %v", err)
	}

	text := string(content)
	truncated := false
	if len(text) > 30000 {
		text = text[:30000]
		truncated = true
	}

	result := OCRResult{
		Source:     source,
		Language:   language,
		Text:       strings.TrimSpace(text),
		Confidence: 0.85, // Tesseract不提供置信度，使用默认值
		Truncated:  truncated,
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化结果失败: %v", err)
	}
	return string(jsonBytes), nil
}

func (t *OCRImageTool) ocrWithPython(imageData []byte, language, source string) (string, error) {
	pyCmd := pythonCmd()

	// 保存临时图片（在 clawdata/tmp 目录）
	tmpImg := getTmpFile("goclaw_ocr_"+fmt.Sprintf("%d", time.Now().UnixNano()), ".png")
	if err := os.WriteFile(tmpImg, imageData, 0644); err != nil {
		return "", fmt.Errorf("保存临时图片失败: %v", err)
	}
	defer os.Remove(tmpImg)

	script := fmt.Sprintf(`
import pytesseract
from PIL import Image
img = Image.open(r'%s')
text = pytesseract.image_to_string(img, lang='%s')
print(text)
`, tmpImg, language)

	cmd := exec.Command(pyCmd, "-c", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Python OCR 失败: %v\n%s", err, string(output))
	}

	text := string(output)
	truncated := false
	if len(text) > 30000 {
		text = text[:30000]
		truncated = true
	}

	result := OCRResult{
		Source:     source,
		Language:   language,
		Text:       strings.TrimSpace(text),
		Confidence: 0.85,
		Truncated:  truncated,
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化结果失败: %v", err)
	}
	return string(jsonBytes), nil
}

func init() {
	GlobalRegistry.Register("ocr_image", func() Tool {
		return NewOCRImageTool()
	})
}