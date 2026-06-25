package tool

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	glog "go-claw/pkg/log"
	"go-claw/pkg/utils"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// BrowserUseTool 浏览器自动化工具
type BrowserUseTool struct{}

var (
	browserMu   sync.Mutex
	browser     *rod.Browser
	currentPage *rod.Page
)

// elementTimeout 元素查找超时时间
const elementTimeout = 10 * time.Second

// navigationTimeout 导航超时时间
const navigationTimeout = 30 * time.Second

// safeCall wraps a rod Must* call with panic recovery
func safeCall(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	fn()
	return nil
}

// isXPath checks if the selector is an XPath expression (starts with / or (//)
func isXPath(selector string) bool {
	return strings.HasPrefix(selector, "/") || strings.HasPrefix(selector, "(")
}

// safeElement 在指定超时内查找元素，返回错误而不是 panic
// 自动检测 XPath 和 CSS 选择器，使用对应的 rod 方法
func safeElement(page *rod.Page, selector string, timeout time.Duration) (*rod.Element, error) {
	var el *rod.Element
	var err error

	if isXPath(selector) {
		// 使用 XPath 方法
		err = safeCall(func() {
			el = page.Timeout(timeout).MustElementX(selector)
		})
	} else {
		// 使用 CSS 选择器方法
		err = safeCall(func() {
			el = page.Timeout(timeout).MustElement(selector)
		})
	}
	if err != nil {
		return nil, err
	}
	return el, nil
}

// resolveSelector converts Playwright-style selectors to rod-compatible selectors
// Rod supports: CSS selectors and XPath (starting with / or //)
func resolveSelector(selector string) (string, error) {
	// Handle simple tag:has-text("...") → XPath conversion
	// e.g., a:has-text("研究报告") → //a[contains(text(), "研究报告")]
	hasTextRe := regexp.MustCompile(`^(\w+):has-text\("(.+)"\)$`)
	if matches := hasTextRe.FindStringSubmatch(selector); len(matches) == 3 {
		tag := matches[1]
		text := matches[2]
		return fmt.Sprintf("//%s[contains(text(), \"%s\")]", tag, text), nil
	}

	// Handle tag.class:has-text("...") → XPath with class
	classHasTextRe := regexp.MustCompile(`^(\w+)\.([\w-]+):has-text\("(.+)"\)$`)
	if matches := classHasTextRe.FindStringSubmatch(selector); len(matches) == 4 {
		tag := matches[1]
		class := matches[2]
		text := matches[3]
		return fmt.Sprintf("//%s[contains(@class, \"%s\") and contains(text(), \"%s\")]", tag, class, text), nil
	}

	// Handle tag#id:has-text("...") → XPath with id
	idHasTextRe := regexp.MustCompile(`^(\w+)#([\w-]+):has-text\("(.+)"\)$`)
	if matches := idHasTextRe.FindStringSubmatch(selector); len(matches) == 4 {
		tag := matches[1]
		id := matches[2]
		text := matches[3]
		return fmt.Sprintf("//%s[@id=\"%s\" and contains(text(), \"%s\")]", tag, id, text), nil
	}

	// More complex :has-text patterns that we can't easily convert
	if strings.Contains(selector, ":has-text(") {
		return "", fmt.Errorf("不支持复杂的 :has-text() 选择器，请使用 XPath 如 //a[contains(text(),\"文本\")]")
	}

	// Other Playwright selectors we don't support
	unsupported := []string{":visible", ":nth-match(", ":first", ":last", "text="}
	for _, s := range unsupported {
		if strings.Contains(selector, s) {
			return "", fmt.Errorf("不支持 Playwright 选择器 '%s'，请使用标准 CSS 或 XPath", s)
		}
	}

	return selector, nil
}

func getBrowser() (*rod.Browser, error) {
	browserMu.Lock()
	if browser != nil {
		browserMu.Unlock()
		return browser, nil
	}
	browserMu.Unlock()

	browserMu.Lock()
	defer browserMu.Unlock()
	// double-check
	if browser != nil {
		return browser, nil
	}

	glog.Logger().Info("[Browser] 正在启动浏览器...")

	// 使用 rod 自动管理浏览器（会自动下载 Chromium 如果需要）
	l := launcher.New()
	l.Headless(true)
	l.NoSandbox(true) // Docker/容器 root 用户必需
	l.Append("--no-first-run", "--no-default-browser-check", "--disable-dev-shm-usage", "--disable-gpu")
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
		glog.Logger().Error("[Browser] 启动失败", "err", err)
		return nil, fmt.Errorf("启动浏览器失败: %v", err)
	}

	var b *rod.Browser
	if err := safeCall(func() {
		b = rod.New().ControlURL(url).MustConnect()
	}); err != nil {
		glog.Logger().Error("[Browser] 连接失败", "err", err)
		return nil, fmt.Errorf("连接浏览器失败: %v", err)
	}

	browser = b
	glog.Logger().Info("[Browser] 浏览器启动成功")
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
				"description": "元素选择器。支持标准 CSS（如 #id, .class, div > a）和 XPath（如 //a[contains(text(),'文本')]）。不支持 Playwright 伪选择器如 :has-text()、:visible。",
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

	var page *rod.Page
	if err := safeCall(func() {
		page = b.Timeout(navigationTimeout).MustPage(url)
	}); err != nil {
		return "", fmt.Errorf("打开页面失败: %v", err)
	}

	if err := safeCall(func() {
		page.Timeout(navigationTimeout).MustWaitLoad()
	}); err != nil {
		return "", fmt.Errorf("等待页面加载超时: %v", err)
	}

	browserMu.Lock()
	currentPage = page
	browserMu.Unlock()

	time.Sleep(2 * time.Second) // 等待JS渲染

	var title string
	if err := safeCall(func() {
		title = page.MustInfo().Title
	}); err != nil {
		return "", fmt.Errorf("获取页面信息失败: %v", err)
	}

	var text string
	if err := safeCall(func() {
		text = page.MustEval("() => document.body ? document.body.innerText : ''").String()
	}); err != nil {
		return "", fmt.Errorf("提取页面内容失败: %v", err)
	}

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

	// 解析并验证选择器
	resolvedSelector, err := resolveSelector(selector)
	if err != nil {
		return "", err
	}

	page, err := t.getCurrentPage()
	if err != nil {
		return "", err
	}

	var el *rod.Element
	el, err = safeElement(page, resolvedSelector, elementTimeout)
	if err != nil {
		return "", fmt.Errorf("查找元素超时: %v (selector: %s)", err, selector)
	}

	if err := safeCall(func() {
		el.MustClick()
	}); err != nil {
		return "", fmt.Errorf("点击失败: %v", err)
	}

	return fmt.Sprintf("已点击: %s", selector), nil
}

func (t *BrowserUseTool) typeText(b *rod.Browser, params map[string]interface{}) (string, error) {
	selector, _ := params["selector"].(string)
	text, _ := params["text"].(string)
	if selector == "" || text == "" {
		return "", fmt.Errorf("需要 selector 和 text 参数")
	}

	// 解析并验证选择器
	resolvedSelector, err := resolveSelector(selector)
	if err != nil {
		return "", err
	}

	page, err := t.getCurrentPage()
	if err != nil {
		return "", err
	}

	var el *rod.Element
	el, err = safeElement(page, resolvedSelector, elementTimeout)
	if err != nil {
		return "", fmt.Errorf("查找元素超时: %v (selector: %s)", err, selector)
	}

	if err := safeCall(func() {
		el.MustSelectAllText()
	}); err != nil {
		return "", fmt.Errorf("选中文本失败: %v", err)
	}

	if err := safeCall(func() {
		el.MustInput(text)
	}); err != nil {
		return "", fmt.Errorf("输入失败: %v", err)
	}

	return fmt.Sprintf("已输入: %s", utils.Truncate(text, 30)), nil
}

func (t *BrowserUseTool) extract(b *rod.Browser, params map[string]interface{}) (string, error) {
	selector, _ := params["selector"].(string)

	page, err := t.getCurrentPage()
	if err != nil {
		return "", err
	}

	var text string
	if selector != "" {
		// 解析并验证选择器
		resolvedSelector, err := resolveSelector(selector)
		if err != nil {
			return "", err
		}

		var el *rod.Element
		el, err = safeElement(page, resolvedSelector, elementTimeout)
		if err != nil {
			return "", fmt.Errorf("查找元素超时: %v (selector: %s)", err, selector)
		}

		if err := safeCall(func() {
			text = el.MustText()
		}); err != nil {
			return "", fmt.Errorf("提取文本失败: %v", err)
		}
	} else {
		if err := safeCall(func() {
			text = page.MustEval("() => document.body ? document.body.innerText : ''").String()
		}); err != nil {
			return "", fmt.Errorf("提取页面内容失败: %v", err)
		}
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
		// 解析并验证选择器
		resolvedSelector, err := resolveSelector(selector)
		if err != nil {
			return "", err
		}

		var el *rod.Element
		el, err = safeElement(page, resolvedSelector, elementTimeout)
		if err != nil {
			return "", fmt.Errorf("查找元素超时: %v (selector: %s)", err, selector)
		}

		if err := safeCall(func() {
			buf, _ = el.Screenshot(proto.PageCaptureScreenshotFormatPng, 100)
		}); err != nil {
			return "", fmt.Errorf("截图失败: %v", err)
		}
	} else {
		if err := safeCall(func() {
			buf, _ = page.Screenshot(true, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng, Quality: &quality})
		}); err != nil {
			return "", fmt.Errorf("截图失败: %v", err)
		}
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

	if err := safeCall(func() {
		page.MustEval(fmt.Sprintf("() => { window.scrollBy(0, %d); return true; }", offset))
	}); err != nil {
		return "", fmt.Errorf("滚动失败: %v", err)
	}

	return fmt.Sprintf("已滚动: %s", dir), nil
}

func (t *BrowserUseTool) wait(b *rod.Browser, params map[string]interface{}) (string, error) {
	selector, _ := params["selector"].(string)
	if selector == "" {
		return "", fmt.Errorf("需要 selector 参数")
	}

	// 解析并验证选择器
	resolvedSelector, err := resolveSelector(selector)
	if err != nil {
		return "", err
	}

	page, err := t.getCurrentPage()
	if err != nil {
		return "", err
	}

	_, err = safeElement(page, resolvedSelector, elementTimeout)
	if err != nil {
		return "", fmt.Errorf("等待元素超时: %v (selector: %s)", err, selector)
	}

	return fmt.Sprintf("元素出现: %s", selector), nil
}

func CloseBrowser() {
	browserMu.Lock()
	defer browserMu.Unlock()
	if browser != nil {
		_ = safeCall(func() {
			browser.MustClose()
		})
		browser = nil
		currentPage = nil
		glog.Logger().Info("[Browser] 浏览器已关闭")
	}
}
