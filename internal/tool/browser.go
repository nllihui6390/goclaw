package tool

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	glog "go-claw/pkg/log"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// BrowserUseTool 浏览器自动化工具
type BrowserUseTool struct{}

var (
	browserMu   sync.Mutex
	browser     *rod.Browser
	browserOnce sync.Once
	currentPage *rod.Page
)

func getBrowser() (*rod.Browser, error) {
	var initErr error
	browserOnce.Do(func() {
		glog.Logger().Info("[Browser] 正在启动浏览器...")

		// 使用 rod 自动管理浏览器（会自动下载 Chromium 如果需要）
		l := launcher.New()
		l.Headless(true)
		l.NoSandbox(true)
		l.Leakless(true) // 更稳定的进程管理

		// 尝试查找已安装的 Chrome
		chromePath, _ := launcher.LookPath()
		if chromePath != "" {
			glog.Logger().Info("[Browser] 使用已安装浏览器", "path", chromePath)
			l.Bin(chromePath)
		} else {
			glog.Logger().Info("[Browser] 将自动下载 Chromium")
		}

		url, err := l.Launch()
		if err != nil {
			initErr = fmt.Errorf("启动浏览器失败: %v", err)
			glog.Logger().Error("[Browser] 启动失败", "err", err)
			return
		}

		browser = rod.New().ControlURL(url).MustConnect()
		glog.Logger().Info("[Browser] 浏览器启动成功")
	})

	if initErr != nil {
		return nil, initErr
	}
	return browser, nil
}

func NewBrowserUseTool() *BrowserUseTool {
	return &BrowserUseTool{}
}

func (t *BrowserUseTool) Name() string {
	return "browser_use"
}

func (t *BrowserUseTool) Description() string {
	return `浏览器自动化工具。navigate 操作会打开URL并返回页面内容。支持 JS 渲染页面。`
}

func (t *BrowserUseTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "操作类型: navigate, click, type, extract, screenshot, scroll, wait",
				"enum":        []string{"navigate", "click", "type", "extract", "screenshot", "scroll", "wait"},
			},
			"url": map[string]interface{}{
				"type":        "string",
				"description": "目标URL",
			},
			"selector": map[string]interface{}{
				"type":        "string",
				"description": "CSS选择器",
			},
			"text": map[string]interface{}{
				"type":        "string",
				"description": "要输入的文本",
			},
		},
		"required": []string{"action"},
	}
}

func (t *BrowserUseTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	logger := glog.Logger()
	action, _ := params["action"].(string)

	logger.Info("[Browser] 执行操作", "action", action)

	b, err := getBrowser()
	if err != nil {
		logger.Error("[Browser] 获取浏览器失败", "err", err)
		return "", err
	}

	switch action {
	case "navigate":
		return t.navigate(b, params)
	case "click":
		return t.click(b, params)
	case "type":
		return t.typeText(b, params)
	case "extract":
		return t.extract(b, params)
	case "screenshot":
		return t.screenshot(b, params)
	case "scroll":
		return t.scroll(b, params)
	case "wait":
		return t.wait(b, params)
	default:
		return "", fmt.Errorf("不支持的操作: %s", action)
	}
}

func (t *BrowserUseTool) navigate(b *rod.Browser, params map[string]interface{}) (string, error) {
	logger := glog.Logger()
	url, _ := params["url"].(string)
	if url == "" {
		return "", fmt.Errorf("需要 url 参数")
	}

	logger.Info("[Browser] 导航开始", "url", url)
	startTime := time.Now()

	page := b.MustPage(url)
	page.MustWaitLoad()

	browserMu.Lock()
	currentPage = page
	browserMu.Unlock()

	time.Sleep(3 * time.Second) // 等待JS渲染

	title := page.MustInfo().Title
	text := page.MustEval("() => document.body ? document.body.innerText : ''").String()

	logger.Info("[Browser] 导航成功", "url", url, "title", title, "content_len", len(text), "elapsed_ms", time.Since(startTime).Milliseconds())

	result := fmt.Sprintf("页面: %s\n标题: %s\n\n", url, title)
	if len(text) > 2000 {
		result += text[:2000] + "\n...(已截断)"
	} else {
		result += text
	}
	return result, nil
}

func (t *BrowserUseTool) getCurrentPage() (*rod.Page, error) {
	browserMu.Lock()
	p := currentPage
	browserMu.Unlock()
	if p == nil {
		return nil, fmt.Errorf("没有当前页面，请先执行 navigate")
	}
	return p, nil
}

func (t *BrowserUseTool) click(b *rod.Browser, params map[string]interface{}) (string, error) {
	selector, _ := params["selector"].(string)
	if selector == "" {
		return "", fmt.Errorf("需要 selector 参数")
	}

	page, err := t.getCurrentPage()
	if err != nil {
		return "", err
	}

	el := page.MustElement(selector)
	el.MustClick()
	return fmt.Sprintf("已点击: %s", selector), nil
}

func (t *BrowserUseTool) typeText(b *rod.Browser, params map[string]interface{}) (string, error) {
	selector, _ := params["selector"].(string)
	text, _ := params["text"].(string)
	if selector == "" || text == "" {
		return "", fmt.Errorf("需要 selector 和 text 参数")
	}

	page, err := t.getCurrentPage()
	if err != nil {
		return "", err
	}

	el := page.MustElement(selector)
	el.MustSelectAllText()
	el.MustInput(text)
	return fmt.Sprintf("已输入: %s", truncate(text, 30)), nil
}

func (t *BrowserUseTool) extract(b *rod.Browser, params map[string]interface{}) (string, error) {
	selector, _ := params["selector"].(string)

	page, err := t.getCurrentPage()
	if err != nil {
		return "", err
	}

	var text string
	if selector != "" {
		el := page.MustElement(selector)
		text = el.MustText()
	} else {
		text = page.MustEval("() => document.body ? document.body.innerText : ''").String()
	}

	if len(text) > 3000 {
		return text[:3000] + "\n...(已截断)", nil
	}
	return text, nil
}

func (t *BrowserUseTool) screenshot(b *rod.Browser, params map[string]interface{}) (string, error) {
	selector, _ := params["selector"].(string)

	page, err := t.getCurrentPage()
	if err != nil {
		return "", err
	}

	var buf []byte
	quality := 90
	if selector != "" {
		el := page.MustElement(selector)
		buf, _ = el.Screenshot(proto.PageCaptureScreenshotFormatPng, 100)
	} else {
		buf, _ = page.Screenshot(true, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng, Quality: &quality})
	}

	return fmt.Sprintf("截图成功 %d bytes\ndata:image/png;base64,%s", len(buf), base64.StdEncoding.EncodeToString(buf)), nil
}

func (t *BrowserUseTool) scroll(b *rod.Browser, params map[string]interface{}) (string, error) {
	dir, _ := params["direction"].(string)
	if dir == "" {
		dir = "down"
	}

	page, err := t.getCurrentPage()
	if err != nil {
		return "", err
	}

	offset := 500
	if dir == "up" {
		offset = -500
	}

	page.MustEval(fmt.Sprintf("() => { window.scrollBy(0, %d); return true; }", offset))
	return fmt.Sprintf("已滚动: %s", dir), nil
}

func (t *BrowserUseTool) wait(b *rod.Browser, params map[string]interface{}) (string, error) {
	selector, _ := params["selector"].(string)
	if selector == "" {
		return "", fmt.Errorf("需要 selector 参数")
	}

	page, err := t.getCurrentPage()
	if err != nil {
		return "", err
	}

	page.MustElement(selector)
	return fmt.Sprintf("元素出现: %s", selector), nil
}

func CloseBrowser() {
	browserMu.Lock()
	defer browserMu.Unlock()
	if browser != nil {
		browser.MustClose()
		browser = nil
		currentPage = nil
		glog.Logger().Info("[Browser] 浏览器已关闭")
	}
}