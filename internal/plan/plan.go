package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TaskStatus 任务状态
type TaskStatus string

const (
	StatusTodo       TaskStatus = "todo"
	StatusInProgress TaskStatus = "in_progress"
	StatusDone       TaskStatus = "done"
	StatusAbandoned  TaskStatus = "abandoned"
)

// SubTask 子任务
type SubTask struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Plan 任务计划
type Plan struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	SubTasks    []SubTask  `json:"sub_tasks"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	mu          sync.RWMutex
	filePath    string
}

// NewPlan 创建任务计划
func NewPlan(description, workspaceDir string) *Plan {
	planDir := workspaceDir + "/plans"
	os.MkdirAll(planDir, 0755)

	p := &Plan{
		ID:          generatePlanID(),
		Description: description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		filePath:    filepath.Join(planDir, fmt.Sprintf("%s.json", time.Now().Format("20060102-150405"))),
	}
	return p
}

// LoadPlan 加载已有计划
func LoadPlan(filePath string) (*Plan, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var p Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	p.filePath = filePath
	return &p, nil
}

// AddSubTask 添加子任务
func (p *Plan) AddSubTask(title, description string) *SubTask {
	p.mu.Lock()
	defer p.mu.Unlock()

	task := SubTask{
		ID:          fmt.Sprintf("task_%d", len(p.SubTasks)+1),
		Title:       title,
		Description: description,
		Status:      StatusTodo,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	p.SubTasks = append(p.SubTasks, task)
	p.UpdatedAt = time.Now()
	p.save()

	return &p.SubTasks[len(p.SubTasks)-1]
}

// UpdateTaskStatus 更新任务状态
func (p *Plan) UpdateTaskStatus(taskID string, status TaskStatus) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, t := range p.SubTasks {
		if t.ID == taskID {
			p.SubTasks[i].Status = status
			p.SubTasks[i].UpdatedAt = time.Now()
			p.UpdatedAt = time.Now()
			p.save()
			return true
		}
	}
	return false
}

// GetNextTask 获取下一个待办任务
func (p *Plan) GetNextTask() *SubTask {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for i, t := range p.SubTasks {
		if t.Status == StatusTodo {
			return &p.SubTasks[i]
		}
	}
	return nil
}

// Progress 获取进度
func (p *Plan) Progress() (todo, inProgress, done, abandoned int) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, t := range p.SubTasks {
		switch t.Status {
		case StatusTodo:
			todo++
		case StatusInProgress:
			inProgress++
		case StatusDone:
			done++
		case StatusAbandoned:
			abandoned++
		}
	}
	return
}

// IsComplete 检查计划是否已完成
func (p *Plan) IsComplete() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, t := range p.SubTasks {
		if t.Status == StatusTodo || t.Status == StatusInProgress {
			return false
		}
	}
	return len(p.SubTasks) > 0
}

// Summary 生成计划摘要
func (p *Plan) Summary() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	todo, inProg, done, abandoned := p.Progress()

	summary := fmt.Sprintf("# 计划: %s\n\n进度: %d 待办 / %d 进行中 / %d 完成 / %d 已放弃\n\n",
		p.Description, todo, inProg, done, abandoned)

	for _, t := range p.SubTasks {
		icon := ""
		switch t.Status {
		case StatusTodo:
			icon = "[ ]"
		case StatusInProgress:
			icon = "[~]"
		case StatusDone:
			icon = "[x]"
		case StatusAbandoned:
			icon = "[-]"
		}
		summary += fmt.Sprintf("%s %s: %s\n", icon, t.Title, t.Description)
	}

	return summary
}

// save 保存计划到文件
func (p *Plan) save() {
	data, _ := json.MarshalIndent(p, "", "  ")
	os.WriteFile(p.filePath, data, 0644)
}

func generatePlanID() string {
	return fmt.Sprintf("plan_%s", time.Now().Format("20060102150405"))
}