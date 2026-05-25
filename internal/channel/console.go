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
	name       string
	msgChan    chan Message
	stopChan   chan struct{}
	ctrlChan   chan ControlResponse // 控制响应通道
	currentAgent string              // 当前选中的Agent
}

// NewConsoleChannel 创建控制台渠道
func NewConsoleChannel() *ConsoleChannel {
	return &ConsoleChannel{
		name:     "console",
		msgChan:  make(chan Message, 100),
		stopChan: make(chan struct{}),
		ctrlChan: make(chan ControlResponse, 10),
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
						Agent:     c.currentAgent, // 携带当前选中的Agent
						Timestamp: timeNow(),
					}
					c.msgChan <- msg
				}
				fmt.Print("> ")
			}
		}
	}
}

// handleCommand 处理控制台命令，返回true表示已处理
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
