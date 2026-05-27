package memory

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go-claw/internal/workspace"
	glog "go-claw/pkg/log"
)

// DreamOptimizer 记忆整理器（空闲时去重、合并、清理 MEMORY.md）
type DreamOptimizer struct {
	wsLoader *workspace.Loader
	runtime  RuntimeCaller
}

// RuntimeCaller LLM 调用接口
type RuntimeCaller interface {
	CallLLM(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// NewDreamOptimizer 创建记忆整理器
func NewDreamOptimizer(wsLoader *workspace.Loader, runtime RuntimeCaller) *DreamOptimizer {
	return &DreamOptimizer{
		wsLoader: wsLoader,
		runtime:  runtime,
	}
}

// Optimize 执行记忆整理
func (d *DreamOptimizer) Optimize(ctx context.Context) error {
	logger := glog.Logger()
	logger.Info("[Dream] 开始记忆整理")

	// 1. 加载当前 MEMORY.md
	currentMemory, err := d.wsLoader.LoadFile("MEMORY.md")
	if err != nil || currentMemory == "" {
		logger.Info("[Dream] MEMORY.md 为空，无需整理")
		return nil
	}

	// 2. 加载最近的每日记忆
	dailyMemory := d.wsLoader.LoadDailyMemory()

	// 3. 调用 LLM 整理记忆
	optimized, err := d.optimizeWithLLM(ctx, currentMemory, dailyMemory)
	if err != nil {
		logger.Warn("[Dream] LLM 整理失败", "err", err)
		return err
	}

	if optimized == "" || optimized == currentMemory {
		logger.Info("[Dream] 记忆无变化，跳过更新")
		return nil
	}

	// 4. 写回 MEMORY.md
	memoryPath := d.wsLoader.GetWorkspaceDir() + "/MEMORY.md"
	if err := writeToFile(memoryPath, optimized); err != nil {
		logger.Warn("[Dream] 写回 MEMORY.md 失败", "err", err)
		return err
	}

	logger.Info("[Dream] 记忆整理完成", "original_len", len(currentMemory), "optimized_len", len(optimized))
	return nil
}

// optimizeWithLLM 调用 LLM 优化记忆
func (d *DreamOptimizer) optimizeWithLLM(ctx context.Context, currentMemory, dailyMemory string) (string, error) {
	systemPrompt := `你是记忆整理助手。请按照以下原则优化用户的长期记忆（MEMORY.md）：

## 极简主义
- 删除日常琐事和重复条目
- 只保留真正重要、长期有用的信息
- 每条信息用简洁的一句话描述

## 状态覆盖
- 新的认知覆盖旧的（如偏好变化）
- 不保留互相矛盾的信息
- 只保留最新状态

## 归纳合并
- 多条相似信息合并为一条更精炼的
- 从具体事例归纳出一般规律
- 去除冗余的细节

## 过期清理
- 明确过期的信息直接删除
- 不确定是否过期的保留但标记 [待确认]

输出优化后的 MEMORY.md 内容，不要添加解释。如果优化后无变化，输出原内容。`

	userPrompt := fmt.Sprintf("当前 MEMORY.md 内容：\n%s\n\n最近每日记忆：\n%s", currentMemory, dailyMemory)

	result, err := d.runtime.CallLLM(ctx, systemPrompt, userPrompt)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(result), nil
}

func writeToFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}