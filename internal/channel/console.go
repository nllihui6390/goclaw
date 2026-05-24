package channel

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

// ConsoleChannel 控制台渠道
type ConsoleChannel struct {
	name     string
	msgChan  chan Message
	stopChan chan struct{}
}

// NewConsoleChannel 创建控制台渠道
func NewConsoleChannel() *ConsoleChannel {
	return &ConsoleChannel{
		name:     "console",
		msgChan:  make(chan Message, 100),
		stopChan: make(chan struct{}),
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
				if text == "/exit" {
					fmt.Println("再见！")
					os.Exit(0)
				}

				if strings.TrimSpace(text) != "" {
					c.msgChan <- Message{
						ID:        fmt.Sprintf("msg-%d", timeNow()),
						Channel:   c.name,
						From:      "user",
						Content:   text,
						Timestamp: timeNow(),
					}
				}
				fmt.Print("> ")
			}
		}
	}
}

func timeNow() int64 {
	return 0 // 简化实现，实际应该用time.Now().Unix()
}
