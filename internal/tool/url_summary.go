package tool

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// URLSummaryTool 网页正文提取工具
type URLSummaryTool struct{}

func NewURLSummaryTool() *URLSummaryTool {
	return &URLSummaryTool{}
}

func (t *URLSummaryTool) Name() string {
	return "url_summary"
}

func (t *URLSummaryTool) Description() string {
	return `提取网页正文内容，过滤广告、导航等噪音，返回干净的文本摘要。
适合用于获取文章、新闻、博客等页面的核心内容。

调用格式：
- url_summary(url="https://example.com/article")  # 提取正文
- url_summary(url="https://example.com", max_length=3000)  # 限制返回长度

参数说明：
- url: 网页地址（必填）
- max_length: 最大返回字符数，默认 5000`
}

func (t *URLSummaryTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "网页地址",
			},
			"max_length": map[string]interface{}{
				"type":        "integer",
				"description": "最大返回字符数，默认5000",
			},
		},
		"required": []string{"url"},
	}
}

func (t *URLSummaryTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	urlStr, ok := params["url"].(string)
	if !ok || urlStr == "" {
		return "", fmt.Errorf("缺少 url 参数")
	}

	maxLength := 5000
	if ml, ok := params["max_length"].(float64); ok && ml > 0 {
		maxLength = int(ml)
	}

	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	html := string(body)

	// 提取标题
	title := extractBetween(html, "<title>", "</title>")
	if title == "" {
		title = urlStr
	}

	// 提取正文
	content := extractMainContent(html)

	// 清理
	content = cleanText(content)

	if len(content) > maxLength {
		content = content[:maxLength] + "\n... [内容过长已截断]"
	}

	return fmt.Sprintf("## %s\n\n来源: %s\n\n%s", title, urlStr, content), nil
}

func extractMainContent(html string) string {
	// 移除 script/style/nav/header/footer 等噪音标签
	html = removeTags(html, "script", "style", "nav", "header", "footer", "aside", "iframe", "noscript")

	// 优先提取 article/main/content 区域
	areas := []string{"<article", "<main", `<div class="content"`, `<div class="article"`, `<div id="content"`, `<div id="article"`}
	for _, area := range areas {
		content := extractTagBlock(html, area)
		if len(content) > 200 {
			return content
		}
	}

	// 回退：提取所有 p 标签内容
	return extractAllParagraphs(html)
}
func removeTags(html string, tagsStr ...string) string {
	for _, tag := range tagsStr {
		// 移除开标签到闭标签
		re := regexp.MustCompile(`<` + tag + `[^>]*>.*?</` + tag + `>`)
		html = re.ReplaceAllString(html, "")
		// 自闭合标签
		re2 := regexp.MustCompile(`<` + tag + `[^>]*>`)
		html = re2.ReplaceAllString(html, "")
	}
	return html
}

func extractTagBlock(html, startTag string) string {
	// 找到开始标签
	i := strings.Index(html, startTag)
	if i < 0 {
		return ""
	}

	// 确定标签名（从 startTag 提取）
	tagName := startTag[1:] // 差掉 <
	if idx := strings.Index(tagName, " "); idx > 0 {
		tagName = tagName[:idx]
	}
	if idx := strings.Index(tagName, `"`); idx > 0 {
		tagName = tagName[:idx]
	}
	if idx := strings.Index(tagName, `>`); idx > 0 {
		tagName = tagName[:idx]
	}

	// 从开始位置找到对应的闭标签
	remainder := html[i:]
	endTag := "</" + tagName + ">"
	j := strings.Index(remainder, endTag)
	if j < 0 {
		return stripTags(remainder)
	}
	return stripTags(remainder[:j+len(endTag)])
}

func extractAllParagraphs(html string) string {
	re := regexp.MustCompile(`<p[^>]*>(.*?)</p>`)
	matches := re.FindAllStringSubmatch(html, -1)
	var parts []string
	for _, m := range matches {
		text := stripTags(m[1])
		if len(text) > 50 {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func cleanText(s string) string {
	// 去除 HTML 标签
	s = stripTags(s)

	// 去除多余空白
	re := regexp.MustCompile(`\n{3,}`)
	s = re.ReplaceAllString(s, "\n\n")

	re2 := regexp.MustCompile(`[ \t]+`)
	s = re2.ReplaceAllString(s, " ")

	// 去除 HTML 实体
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")

	return strings.TrimSpace(s)
}

func init() {
	GlobalRegistry.Register("url_summary", func() Tool {
		return NewURLSummaryTool()
	})
}
