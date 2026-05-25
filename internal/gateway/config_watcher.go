package gateway

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"go-claw/pkg/log"
)

// ConfigWatcher 配置文件监听器
type ConfigWatcher struct {
	path    string
	onChange func()
	stopCh  chan struct{}
	mu      sync.Mutex
}

// NewConfigWatcher 创建配置监听器
func NewConfigWatcher(path string, onChange func()) *ConfigWatcher {
	return &ConfigWatcher{
		path:     path,
		onChange: onChange,
		stopCh:   make(chan struct{}),
	}
}

// Start 开始监听
func (w *ConfigWatcher) Start() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	dir := filepath.Dir(w.path)
	if err := watcher.Add(dir); err != nil {
		return err
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case <-w.stopCh:
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Name == w.path && (event.Op&fsnotify.Write == fsnotify.Write ||
					event.Op&fsnotify.Create == fsnotify.Create) {
					// 防抖：等待300ms确保写入完成
					time.Sleep(300 * time.Millisecond)
					w.mu.Lock()
					if w.onChange != nil {
						log.Logger().Info("配置文件已变更，正在重新加载", "path", w.path)
						w.onChange()
					}
					w.mu.Unlock()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Logger().Error("配置监听错误", "err", err)
			}
		}
	}()

	log.Logger().Info("配置文件监听已启动", "path", w.path)
	return nil
}

// Stop 停止监听
func (w *ConfigWatcher) Stop() {
	close(w.stopCh)
}

// HotReloadEnabled 检查是否启用热加载
func HotReloadEnabled() bool {
	return os.Getenv("GOCLAW_HOT_RELOAD") == "true"
}
