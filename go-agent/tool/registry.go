package tool

// =============================================
// Registry 扩展方法
//
// 此文件提供 Registry 的额外便利方法：
//   - Merge: 合并另一个注册表的所有工具
//   - Clone: 深克隆当前注册表
//   - HasTool/Remove/Count/Names: 查询和统计方法
//   - GlobalRegistry: 旧式的全局注册表（推荐使用依赖注入）
// =============================================

// Merge 合并另一个注册表的所有工具、工厂和分组。
//
// 参数：
//   - other: 要合并的注册表
func (r *Registry) Merge(other *Registry) {
	r.mu.Lock(); defer r.mu.Unlock()
	other.mu.RLock(); defer other.mu.RUnlock()
	for name, t := range other.tools   { r.tools[name] = t }
	for name, factory := range other.factories { r.factories[name] = factory }
	for group, names := range other.groups    { r.groups[group] = names }
}

// Clone 深克隆当前注册表。
//
// 返回：
//   - *Registry: 新的独立注册表副本
func (r *Registry) Clone() *Registry {
	r.mu.RLock(); defer r.mu.RUnlock()
	newReg := NewRegistry()
	for name, t := range r.tools     { newReg.tools[name] = t }
	for name, f := range r.factories  { newReg.factories[name] = f }
	for group, names := range r.groups { newReg.groups[group] = names }
	return newReg
}

// HasTool 检查指定工具是否存在。
//
// 参数：
//   - name: 工具名称
//
// 返回：
//   - bool: 是否存在
func (r *Registry) HasTool(name string) bool {
	r.mu.RLock(); defer r.mu.RUnlock()
	if _, ok := r.tools[name]; ok { return true }
	_, ok := r.factories[name]; return ok
}

// Remove 移除工具（同时移除直接注册和工厂）。
//
// 参数：
//   - name: 工具名称
func (r *Registry) Remove(name string) {
	r.mu.Lock(); defer r.mu.Unlock()
	delete(r.tools, name); delete(r.factories, name)
}

// Count 获取工具总数（直接注册 + 工厂）。
//
// 返回：
//   - int: 工具总数
func (r *Registry) Count() int {
	r.mu.RLock(); defer r.mu.RUnlock()
	return len(r.tools) + len(r.factories)
}

// Names 获取所有工具名称列表。
//
// 返回：
//   - []string: 工具名称列表
func (r *Registry) Names() []string {
	r.mu.RLock(); defer r.mu.RUnlock()
	names := make(map[string]bool)
	for name := range r.tools     { names[name] = true }
	for name := range r.factories  { names[name] = true }
	result := make([]string, 0, len(names))
	for name := range names { result = append(result, name) }
	return result
}

// =============================================
// GlobalRegistry — 旧式全局注册表（不推荐，推荐依赖注入）
// =============================================

// GlobalRegistry 全局工具注册表（不推荐使用）。
// 推荐通过 Config 依赖注入 Registry 实例，避免全局状态。
var GlobalRegistry = NewRegistry()

// RegisterGlobal 注册工具到全局注册表。
//
// 参数：
//   - tool: 要注册的工具
func RegisterGlobal(tool Tool) { GlobalRegistry.Register(tool) }

// GetGlobal 从全局注册表获取工具。
//
// 参数：
//   - name: 工具名称
//
// 返回：
//   - Tool: 工具实例
//   - bool: 是否找到
func GetGlobal(name string) (Tool, bool) { return GlobalRegistry.Get(name) }