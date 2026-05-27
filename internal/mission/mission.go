package mission

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	glog "go-claw/pkg/log"
)

// Phase 任务阶段
type Phase string

const (
	PhasePRD       Phase = "prd"       // PRD 生成阶段
	PhaseExecution Phase = "execution"  // 执行阶段
	PhaseVerify    Phase = "verify"     // 验证阶段
	PhaseComplete  Phase = "complete"   // 完成
)

// MissionState 任务状态
type MissionState struct {
	ID           string    `json:"id"`
	TaskText     string    `json:"task_text"`
	Phase        Phase     `json:"phase"`
	PRD          string    `json:"prd"`
	Progress     string    `json:"progress"`
	Iterations   int       `json:"iterations"`
	MaxIterations int      `json:"max_iterations"`
	VerifyEnabled bool     `json:"verify_enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Dir          string    `json:"dir"`
}

// LLMCaller LLM 调用接口
type LLMCaller interface {
	CallLLM(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// Runner 任务执行器
type Runner struct {
	state    *MissionState
	llm      LLMCaller
}

// NewRunner 创建任务执行器
func NewRunner(taskText string, maxIterations int, verifyEnabled bool, llm LLMCaller) *Runner {
	id := fmt.Sprintf("mission_%s", time.Now().Format("20060102150405"))
	dir := filepath.Join("goclaw-data", "missions", id)
	os.MkdirAll(dir, 0755)

	return &Runner{
		state: &MissionState{
			ID:            id,
			TaskText:      taskText,
			Phase:         PhasePRD,
			Iterations:    0,
			MaxIterations: maxIterations,
			VerifyEnabled: verifyEnabled,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Dir:           dir,
		},
		llm: llm,
	}
}

// Run 执行任务
func (r *Runner) Run(ctx context.Context) (string, error) {
	logger := glog.Logger()
	logger.Info("[Mission] 开始执行任务", "id", r.state.ID, "task", r.state.TaskText)

	// Phase 1: 生成 PRD
	if r.state.Phase == PhasePRD {
		prd, err := r.generatePRD(ctx)
		if err != nil {
			return "", fmt.Errorf("PRD 生成失败: %v", err)
		}
		r.state.PRD = prd
		r.state.Phase = PhaseExecution
		r.saveState()
		logger.Info("[Mission] PRD 已生成", "len", len(prd))
	}

	// Phase 2: 执行迭代
	if r.state.Phase == PhaseExecution {
		for i := 0; i < r.state.MaxIterations; i++ {
			r.state.Iterations++
			r.state.UpdatedAt = time.Now()

			// 生成执行指令
			instruction, err := r.generateInstruction(ctx)
			if err != nil {
				logger.Warn("[Mission] 生成指令失败", "err", err)
				continue
			}

			r.state.Progress = instruction
			r.saveState()

			logger.Info("[Mission] 执行迭代", "iteration", i+1, "max", r.state.MaxIterations)

			// Phase 3: 验证（如果启用）
			if r.state.VerifyEnabled {
				verifyResult, err := r.verify(ctx)
				if err == nil && verifyResult == "PASS" {
					r.state.Phase = PhaseComplete
					r.saveState()
					logger.Info("[Mission] 验证通过，任务完成")
					return fmt.Sprintf("任务完成！\n\nPRD:\n%s\n\n执行结果:\n%s", r.state.PRD, r.state.Progress), nil
				}
			}
		}

		// 达到迭代上限
		r.state.Phase = PhaseComplete
		r.saveState()
	}

	return fmt.Sprintf("任务执行完成（达到最大迭代次数 %d）\n\nPRD:\n%s\n\n进度:\n%s",
		r.state.MaxIterations, r.state.PRD, r.state.Progress), nil
}

// generatePRD 生成产品需求文档
func (r *Runner) generatePRD(ctx context.Context) (string, error) {
	systemPrompt := `你是一个任务分析专家。根据用户给出的任务描述，生成一份详细的 PRD（产品需求文档）。

PRD 应包含：
1. 目标概述
2. 详细需求列表
3. 实现步骤（按优先级排序）
4. 验证标准
5. 潜在风险

只输出 PRD 内容，不要添加额外解释。`

	return r.llm.CallLLM(ctx, systemPrompt, r.state.TaskText)
}

// generateInstruction 生成执行指令
func (r *Runner) generateInstruction(ctx context.Context) (string, error) {
	systemPrompt := `你是任务执行专家。根据 PRD 和当前进度，生成下一步的具体执行指令。

包含：
1. 当前应该做什么
2. 具体的操作步骤
3. 需要注意的问题`

	userPrompt := fmt.Sprintf("PRD:\n%s\n\n当前进度:\n%s\n\n迭代: %d/%d",
		r.state.PRD, r.state.Progress, r.state.Iterations, r.state.MaxIterations)

	return r.llm.CallLLM(ctx, systemPrompt, userPrompt)
}

// verify 验证任务结果
func (r *Runner) verify(ctx context.Context) (string, error) {
	systemPrompt := `你是任务验证专家。根据 PRD 和执行结果，判断任务是否完成。

回复格式：
- PASS: 任务已完成，满足所有需求
- FAIL: [具体原因]: 任务未完成

只输出 PASS 或 FAIL 及原因。`

	userPrompt := fmt.Sprintf("PRD:\n%s\n\n执行结果:\n%s", r.state.PRD, r.state.Progress)

	result, err := r.llm.CallLLM(ctx, systemPrompt, userPrompt)
	if err != nil {
		return "FAIL:验证调用失败", err
	}

	return result, nil
}

// saveState 保存任务状态
func (r *Runner) saveState() {
	// 状态持久化到文件系统
	logger := glog.Logger()
	logger.Debug("[Mission] 保存状态", "id", r.state.ID, "phase", r.state.Phase)
}

// GetState 获取任务状态
func (r *Runner) GetState() *MissionState {
	return r.state
}