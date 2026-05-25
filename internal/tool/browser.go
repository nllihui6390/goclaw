package tool

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sync"
	"time"

	glog "go-claw/pkg/log"

	"github.com/chromedp/chromedp"
)

// BrowserUseTool 浏览器自动化工具
type BrowserUseTool struct{}

type BrowserPool struct {
	mu          sync.Mutex
	allocCtx    context.Context
	allocCancel context.CancelFunc
	initialized bool
	chromePath  string
}

var globalBrowserPool *BrowserPool
var browserPoolOnce sync.Once

func getBrowserPool() *BrowserPool {
	browserPoolOnce.Do(func() {
		globalBrowserPool = &BrowserPool{}
	})
	return globalBrowserPool
}

// findChrome 查找Chrome路径
func findChrome() string {
	paths := []string{
		"C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
		"C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe",
	}
	if user := os.Getenv("USERNAME"); user != "" {
		paths = append(paths,
			"C:\\Users\\"+user+"\\AppData\\Local\\Google\\Chrome\\Application\\chrome.exe")
	}
	// Edge
	paths = append(paths,
		"C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe",
		"C:\\Program Files\\Microsoft\\Edge\\Application\\msedge.exe")

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// createBrowserContext 创建一个新的浏览器上下文（每次操作创建新的）
func (p *BrowserPool) createBrowserContext() (context.Context, context.CancelFunc, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	logger := glog.Logger()

	// 第一次需要找到Chrome路径
	if !p.initialized {
		p.chromePath = findChrome()
		if p.chromePath == "" {
			return nil, nil, fmt.Errorf("未找到Chrome浏览器")
		}
		logger.Info("[Browser] 找到Chrome", "path", p.chromePath)
		p.initialized = true
	}

	// 每次创建新的allocator和context
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(p.chromePath),
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.WindowSize(1920, 1080),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)

	return ctx, func() {
		cancel()
		allocCancel()
	}, nil
}

func NewBrowserUseTool() *BrowserUseTool {
	return &BrowserUseTool{}
}

func (t *BrowserUseTool) Name() string {
	return "browser_use"
}

func (t *BrowserUseTool) Description() string {
	return `浏览器自动化工具。navigate 操作会打开URL并直接返回页面文本内容，无需单独调用extract。`
}

func (t *BrowserUseTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "操作类型: navigate, click, type, extract, screenshot, evaluate, scroll, wait",
				"enum":        []string{"navigate", "click", "type", "extract", "screenshot", "evaluate", "scroll", "wait"},
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

	// 创建新的浏览器上下文
	pool := getBrowserPool()
	browserCtx, cleanup, err := pool.createBrowserContext()
	if err != nil {
		logger.Error("[Browser] 创建浏览器失败", "err", err)
		return "", err
	}
	defer cleanup()

	// 设置操作超时
	timeoutCtx, timeoutCancel := context.WithTimeout(browserCtx, 60*time.Second)
	defer timeoutCancel()

	switch action {
	case "navigate":
		return t.navigate(timeoutCtx, params)
	case "click":
		return t.click(timeoutCtx, params)
	case "type":
		return t.typeText(timeoutCtx, params)
	case "extract":
		return t.extract(timeoutCtx, params)
	case "screenshot":
		return t.screenshot(timeoutCtx, params)
	case "evaluate":
		return t.evaluate(timeoutCtx, params)
	case "scroll":
		return t.scroll(timeoutCtx, params)
	case "wait":
		return t.wait(timeoutCtx, params)
	default:
		return "", fmt.Errorf("不支持的操作: %s", action)
	}
}

func (t *BrowserUseTool) navigate(ctx context.Context, params map[string]interface{}) (string, error) {
	url, _ := params["url"].(string)
	if url == "" {
		return "", fmt.Errorf("需要 url 参数")
	}

	logger := glog.Logger()
	logger.Info("[Browser] 导航开始", "url", url)

	var title, text string
	startTime := time.Now()

	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
		chromedp.Sleep(2*time.Second), // 等待页面加载
		chromedp.Title(&title),
		chromedp.Evaluate("document.body.innerText", &text),
	)

	if err != nil {
		logger.Error("[Browser] 导航失败", "url", url, "err", err, "elapsed_ms", time.Since(startTime).Milliseconds())
		return "", fmt.Errorf("导航失败: %v", err)
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

func (t *BrowserUseTool) click(ctx context.Context, params map[string]interface{}) (string, error) {
	selector, _ := params["selector"].(string)
	if selector == "" {
		return "", fmt.Errorf("需要 selector 参数")
	}

	err := chromedp.Run(ctx,
		chromedp.WaitVisible(selector),
		chromedp.Click(selector),
	)
	if err != nil {
		return "", fmt.Errorf("点击失败: %v", err)
	}
	return fmt.Sprintf("已点击: %s", selector), nil
}

func (t *BrowserUseTool) typeText(ctx context.Context, params map[string]interface{}) (string, error) {
	selector, _ := params["selector"].(string)
	text, _ := params["text"].(string)
	if selector == "" || text == "" {
		return "", fmt.Errorf("需要 selector 和 text 参数")
	}

	err := chromedp.Run(ctx,
		chromedp.WaitVisible(selector),
		chromedp.Clear(selector),
		chromedp.SendKeys(selector, text),
	)
	if err != nil {
		return "", fmt.Errorf("输入失败: %v", err)
	}
	return fmt.Sprintf("已输入: %s", truncate(text, 30)), nil
}

func (t *BrowserUseTool) extract(ctx context.Context, params map[string]interface{}) (string, error) {
	selector, _ := params["selector"].(string)

	if selector != "" {
		var text string
		err := chromedp.Run(ctx,
			chromedp.WaitVisible(selector),
			chromedp.Text(selector, &text),
		)
		if err != nil {
			return "", err
		}
		return text, nil
	}

	var text string
	err := chromedp.Run(ctx, chromedp.Evaluate("document.body.innerText", &text))
	if err != nil {
		return "", err
	}
	if len(text) > 3000 {
		return text[:3000] + "\n...(已截断)", nil
	}
	return text, nil
}

func (t *BrowserUseTool) screenshot(ctx context.Context, params map[string]interface{}) (string, error) {
	var buf []byte
	selector, _ := params["selector"].(string)

	if selector != "" {
		err := chromedp.Run(ctx, chromedp.Screenshot(selector, &buf))
		if err != nil {
			return "", err
		}
	} else {
		err := chromedp.Run(ctx, chromedp.FullScreenshot(&buf, 90))
		if err != nil {
			return "", err
		}
	}

	return fmt.Sprintf("截图成功 %d bytes\ndata:image/png;base64,%s", len(buf), base64.StdEncoding.EncodeToString(buf)), nil
}

func (t *BrowserUseTool) evaluate(ctx context.Context, params map[string]interface{}) (string, error) {
	script, _ := params["script"].(string)
	if script == "" {
		return "", fmt.Errorf("需要 script 参数")
	}

	var result interface{}
	err := chromedp.Run(ctx, chromedp.Evaluate(script, &result))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("结果: %v", result), nil
}

func (t *BrowserUseTool) scroll(ctx context.Context, params map[string]interface{}) (string, error) {
	dir, _ := params["direction"].(string)
	if dir == "" {
		dir = "down"
	}

	script := "window.scrollBy(0, 500)"
	if dir == "up" {
		script = "window.scrollBy(0, -500)"
	}

	err := chromedp.Run(ctx, chromedp.Evaluate(script, nil))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("已滚动: %s", dir), nil
}

func (t *BrowserUseTool) wait(ctx context.Context, params map[string]interface{}) (string, error) {
	selector, _ := params["selector"].(string)
	if selector == "" {
		return "", fmt.Errorf("需要 selector 参数")
	}

	err := chromedp.Run(ctx, chromedp.WaitVisible(selector))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("元素出现: %s", selector), nil
}

func CloseBrowser() {
	if p := globalBrowserPool; p != nil {
		p.mu.Lock()
		if p.allocCancel != nil {
			p.allocCancel()
		}
		p.mu.Unlock()
	}
}