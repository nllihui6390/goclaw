package tool

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ReadPDFTool PDF读取工具
type ReadPDFTool struct{}

func NewReadPDFTool() *ReadPDFTool {
	return &ReadPDFTool{}
}

func (t *ReadPDFTool) Name() string {
	return "read_pdf"
}

func (t *ReadPDFTool) Description() string {
	return `读取 PDF 文件内容并提取文本。
支持多页 PDF，返回带页码标记的文本内容。

调用格式：
- read_pdf(path="document.pdf")  # 读取全部页面
- read_pdf(path="document.pdf", pages="1-5")  # 读取指定页码范围
- read_pdf(path="document.pdf", pages="1,3,5")  # 读取指定页面

参数说明：
- path: PDF 文件路径（必填）
- pages: 页码范围，如 "1-5" 或 "1,3,5"，默认全部

依赖：
- 需要 pdftotext 工具（poppler-utils）
- Windows 可从 https://github.com/oschwartz10612/poppler-windows/releases 下载`
}

func (t *ReadPDFTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "PDF 文件路径",
			},
			"pages": map[string]interface{}{
				"type":        "string",
				"description": "页码范围，如 1-5 或 1,3,5",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadPDFTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("缺少 path 参数")
	}

	// 检查文件是否存在
	_, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("文件不存在: %v", err)
	}

	// 安全检查
	if isSensitivePath(path) {
		return "", fmt.Errorf("禁止读取敏感路径: %s", path)
	}

	pages, _ := params["pages"].(string)

	// 尝试使用 pdftotext
	if _, err := exec.LookPath("pdftotext"); err == nil {
		return t.extractWithPdftotext(path, pages)
	}

	// 尝试使用 Python PyPDF2
	if t.hasPythonPDF() {
		return t.extractWithPython(path, pages)
	}

	return "", fmt.Errorf("未找到 PDF 解析工具，请安装 pdftotext 或 Python PyPDF2")
}

func (t *ReadPDFTool) extractWithPdftotext(path, pages string) (string, error) {
	// 创建临时输出文件（在 clawdata/tmp 目录）
	tmpFile := getTmpFile("pdf_"+fmt.Sprintf("%d", os.Getpid()), ".txt")
	defer os.Remove(tmpFile)

	args := []string{"-layout", path, tmpFile}

	// 处理页码范围
	if pages != "" {
		if strings.Contains(pages, "-") {
			parts := strings.Split(pages, "-")
			if len(parts) == 2 {
				args = []string{"-f", parts[0], "-l", parts[1], "-layout", path, tmpFile}
			}
		}
	}

	cmd := exec.Command("pdftotext", args...)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftotext 执行失败: %v", err)
	}

	content, err := os.ReadFile(tmpFile)
	if err != nil {
		return "", fmt.Errorf("读取提取结果失败: %v", err)
	}

	if len(content) == 0 {
		return "PDF 内容为空或无法提取文本", nil
	}

	result := fmt.Sprintf("## PDF 内容: %s\n\n", filepath.Base(path))
	result += string(content)

	if len(result) > 50000 {
		result = result[:50000] + "\n... [内容过长已截断]"
	}

	return result, nil
}

func (t *ReadPDFTool) hasPythonPDF() bool {
	pyCmd := "python3"
	if _, err := exec.LookPath("python3"); err != nil {
		pyCmd = "python"
		if _, err := exec.LookPath("python"); err != nil {
			return false
		}
	}

	// 检查 PyPDF2 是否安装
	cmd := exec.Command(pyCmd, "-c", "import PyPDF2")
	return cmd.Run() == nil
}

func (t *ReadPDFTool) extractWithPython(path, pages string) (string, error) {
	pyCmd := "python3"
	if _, err := exec.LookPath("python3"); err != nil {
		pyCmd = "python"
	}

	script := `
import PyPDF2
import sys

path = sys.argv[1]
pages_str = sys.argv[2] if len(sys.argv) > 2 else ""

with open(path, 'rb') as f:
    reader = PyPDF2.PdfReader(f)
    total = len(reader.pages)

    if pages_str:
        # 解析页码
        pages_to_read = []
        if '-' in pages_str:
            parts = pages_str.split('-')
            start, end = int(parts[0])-1, int(parts[1])
            pages_to_read = range(start, min(end, total))
        else:
            pages_to_read = [int(p)-1 for p in pages_str.split(',') if int(p) <= total]
    else:
        pages_to_read = range(total)

    for i in pages_to_read:
        if 0 <= i < total:
            text = reader.pages[i].extract_text()
            print(f"\n--- 第 {i+1} 页 ---\n{text}")
`

	cmd := exec.Command(pyCmd, "-c", script, path, pages)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Python PDF 提取失败: %v\n%s", err, string(output))
	}

	result := fmt.Sprintf("## PDF 内容: %s\n\n", filepath.Base(path))
	result += string(output)

	if len(result) > 50000 {
		result = result[:50000] + "\n... [内容过长已截断]"
	}

	return result, nil
}

func init() {
	GlobalRegistry.Register("read_pdf", func() Tool {
		return NewReadPDFTool()
	})
}