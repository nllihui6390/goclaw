package channel

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// ConsoleChannel 控制台渠道
type ConsoleChannel struct {
	name         string
	msgChan      chan Message
	stopChan     chan struct{}
	ctrlChan     chan ControlResponse // 控制响应通道
	currentAgent string               // 当前选中的Agent
	display      DisplayConfig        // 显示控制配置
}

// NewConsoleChannel 创建控制台渠道
func NewConsoleChannel(display DisplayConfig) *ConsoleChannel {
	return &ConsoleChannel{
		name:     "console",
		msgChan:  make(chan Message, 100),
		stopChan: make(chan struct{}),
		ctrlChan: make(chan ControlResponse, 10),
		display:  display,
	}
}

func (c *ConsoleChannel) GetName() string {
	return c.name
}

func (c *ConsoleChannel) Start(ctx context.Context) error {
	go c.readLoop(ctx)
	return nil
}

func (c *ConsoleChannel) Stop() error {
	close(c.stopChan)
	return nil
}

func (c *ConsoleChannel) Receive(ctx context.Context) (<-chan Message, error) {
	return c.msgChan, nil
}

func (c *ConsoleChannel) Send(ctx context.Context, resp Response) error {
	fmt.Printf("\n[Assistant] %s\n", resp.Content)
	fmt.Print("> ")
	return nil
}

// SendProactive 主动发送消息到控制台
func (c *ConsoleChannel) SendProactive(ctx context.Context, userID, content string) error {
	fmt.Printf("\n[Proactive] %s\n", content)
	fmt.Print("> ")
	return nil
}

// SendToolEvent 发送工具执行事件（根据显示配置过滤）
func (c *ConsoleChannel) SendToolEvent(event ToolEvent) error {
	if !c.display.ShouldShowToolEvent(event.Type) {
		return nil
	}

	switch event.Type {
	case "thinking":
		if event.Thinking != "" {
			fmt.Printf("\n  💭 思考: %s\n", event.Thinking)
		}
	case "calling":
		fmt.Printf("\n  🔧 调用工具: %s\n", event.ToolName)
		if event.Args != "" {
			args := event.Args
			if len(args) > 200 {
				args = args[:200] + "..."
			}
			fmt.Printf("     参数: %s\n", args)
		}
	case "result":
		result := event.Result
		if len(result) > 500 {
			result = result[:500] + "...(已截断)"
		}
		fmt.Printf("  ✅ 结果: %s\n", result)
	case "error":
		fmt.Printf("  ❌ 错误: %s - %s\n", event.ToolName, event.Error)
	}
	return nil
}

// SendCtrl 发送控制响应
func (c *ConsoleChannel) SendCtrl(msg string) {
	fmt.Printf("\n[系统] %s\n", msg)
	fmt.Print("> ")
}

func (c *ConsoleChannel) readLoop(ctx context.Context) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		default:
			if scanner.Scan() {
				text := scanner.Text()

				if c.handleCommand(text) {
					fmt.Print("> ")
					continue
				}

				if text == "/exit" {
					fmt.Println("再见！")
					os.Exit(0)
				}

				if strings.TrimSpace(text) != "" {
					msg := Message{
						ID:        fmt.Sprintf("msg-%d", timeNow()),
						Channel:   c.name,
						From:      "user",
						Content:   text,
						Agent:     c.currentAgent,
						Timestamp: timeNow(),
					}
					c.msgChan <- msg
				}
				fmt.Print("> ")
			}
		}
	}
}

func (c *ConsoleChannel) handleCommand(text string) bool {
	if !strings.HasPrefix(text, "/") {
		return false
	}

	parts := strings.Fields(text[1:])
	if len(parts) == 0 {
		return false
	}

	cmd := strings.ToLower(parts[0])
	arg := ""
	if len(parts) > 1 {
		arg = strings.Join(parts[1:], " ")
	}

	switch cmd {
	case "agent":
		if arg == "" {
			if c.currentAgent == "" {
				c.SendCtrl("当前使用默认Agent (default)")
			} else {
				c.SendCtrl(fmt.Sprintf("当前Agent: %s", c.currentAgent))
			}
		} else {
			c.currentAgent = arg
			c.SendCtrl(fmt.Sprintf("已切换到Agent: %s", arg))
		}
	case "agents":
		c.SendCtrl("可用Agent列表: default (当前只有一个Agent)")
	case "help":
		c.SendCtrl("可用命令:\n" +
			"  /agent [name]   - 切换Agent（不加参数显示当前Agent）\n" +
			"  /agents         - 列出可用Agent\n" +
			"  /exit           - 退出\n" +
			"  /help           - 显示帮助\n" +
			"\n直接输入内容即可与当前Agent对话")
	default:
		c.SendCtrl(fmt.Sprintf("未知命令: /%s，输入 /help 查看帮助", cmd))
	}

	return true
}

func timeNow() int64 {
	return time.Now().Unix()
}