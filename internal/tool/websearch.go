package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WebSearchTool 网页搜索工具
type WebSearchTool struct{}

func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{}
}

func (t *WebSearchTool) Name() string {
	return "web_search"
}

func (t *WebSearchTool) Description() string {
	return `使用搜索引擎搜索互联网信息。
返回搜索结果列表，包含标题、摘要和链接。

调用格式：
- web_search(query="Go语言并发编程")  # 搜索关键词
- web_search(query="最新AI新闻", count=10)  # 指定结果数量

参数说明：
- query: 搜索关键词（必填）
- count: 返回结果数量，默认 5，最大 10

支持的搜索引擎：
- Bing (需设置 BING_API_KEY 环境变量)
- Sogou (无需 API Key，免费使用)`
}

func (t *WebSearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "搜索关键词",
			},
			"count": map[string]interface{}{
				"type":        "integer",
				"description": "结果数量，默认5，最大10",
			},
		},
		"required": []string{"query"},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("缺少 query 参数")
	}

	count := 5
	if c, ok := params["count"].(float64); ok && c > 0 {
		count = int(c)
		if count > 10 {
			count = 10
		}
	}

	// 尝试 Bing API
	bingKey := getEnv("BING_API_KEY", "")
	if bingKey != "" {
		return searchBing(query, count, bingKey)
	}

	// 回退到 Sogou
	return searchSogou(query, count)
}

func searchBing(query string, count int, apiKey string) (string, error) {
	endpoint := fmt.Sprintf("https://api.bing.microsoft.com/v7.0/search?q=%s&count=%d&mkt=zh-CN",
		url.QueryEscape(query), count)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Bing 搜索请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	var result bingResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	if len(result.WebPages.Value) == 0 {
		return "未找到相关搜索结果", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## 搜索结果: %s\n\n", query))
	sb.WriteString("| # | 标题 | 摘要 | 链接 |\n")
	sb.WriteString("|---|------|------|------|\n")

	for i, item := range result.WebPages.Value {
		if i >= count {
			break
		}
		snippet := item.Snippet
		if len(snippet) > 120 {
			snippet = snippet[:120] + "..."
		}
		sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s |\n", i+1, item.Name, snippet, item.URL))
	}
	return sb.String(), nil
}

func searchSogou(query string, count int) (string, error) {
	searchURL := fmt.Sprintf("https://www.sogou.com/web?query=%s&num=%d", url.QueryEscape(query), count)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Sogou 搜索请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	// 从 HTML 中提取搜索结果
	results := extractSogouResults(string(body), count)

	if len(results) == 0 {
		return "未找到相关搜索结果，建议配置 Bing API Key 获取更精确的结果", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## 搜索结果: %s\n\n", query))
	sb.WriteString("| # | 标题 | 摘要 | 链接 |\n")
	sb.WriteString("|---|------|------|------|\n")

	for i, r := range results {
		snippet := r.Snippet
		if len(snippet) > 120 {
			snippet = snippet[:120] + "..."
		}
		sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s |\n", i+1, r.Title, snippet, r.URL))
	}
	return sb.String(), nil
}

// sogouResult 搜狗搜索结果
type sogouResult struct {
	Title   string
	Snippet string
	URL     string
}

func extractSogouResults(html string, max int) []sogouResult {
	var results []sogouResult
	// 从搜狗 HTML 提取结果块
	blocks := extractBetweenAll(html, `<div class="vrwrap">`, `</div>`)
	for i, block := range blocks {
		if i >= max {
			break
		}
		title := stripTags(extractBetween(block, `<h3>`, `</h3>`))
		snippet := stripTags(extractBetween(block, `<p class="str_info">`, `</p>`))
		link := extractBetween(block, `href="`, `"`)
		if title == "" && snippet == "" {
			continue
		}
		results = append(results, sogouResult{Title: title, Snippet: snippet, URL: link})
	}
	return results
}

// bingResponse Bing API 响应结构
type bingResponse struct {
	WebPages struct {
		Value []bingWebPage `json:"value"`
	} `json:"webPages"`
}

type bingWebPage struct {
	Name     string `json:"name"`
	Snippet  string `json:"snippet"`
	URL      string `json:"url"`
}

// HTML 提取辅助函数
func extractBetween(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	s = s[i+len(start):]
	j := strings.Index(s, end)
	if j < 0 {
		return s
	}
	return s[:j]
}

func extractBetweenAll(s, start, end string) []string {
	var results []string
	for {
		i := strings.Index(s, start)
		if i < 0 {
			break
		}
		s = s[i+len(start):]
		j := strings.Index(s, end)
		if j < 0 {
			results = append(results, s)
			break
		}
		results = append(results, s[:j])
		s = s[j+len(end):]
	}
	return results
}

func stripTags(s string) string {
	var result strings.Builder
	inTag := false
	for _, c := range s {
		if c == '<' {
			inTag = true
			continue
		}
		if c == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(c)
		}
	}
	return strings.TrimSpace(result.String())
}

func init() {
	GlobalRegistry.Register("web_search", func() Tool {
		return NewWebSearchTool()
	})
}