package agent

// wantsCreateSkill 检测用户消息是否涉及创建技能
func wantsCreateSkill(msg string) bool {
	lower := toLower(msg)
	hints := []string{"创建技能", "创建skill", "新建技能", "新建skill", "添加技能", "添加skill",
		"创建一个技能", "创建一个skill", "写一个技能", "写一个skill",
		"做成技能", "做成skill", "保存为技能", "保存为skill",
		"固化成技能", "固化成skill", "封装成技能", "封装成skill",
		"make a skill", "create skill", "new skill", "add skill"}
	for _, h := range hints {
		if contains(lower, h) {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i, c := range s {
		if c >= 'A' && c <= 'Z' {
			result[i] = byte(c + 32)
		} else {
			result[i] = byte(c)
		}
	}
	return string(result)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// getSkillCreationTemplate 返回技能创建提示模板（仅在用户需要创建技能时动态注入）
func getSkillCreationTemplate() string {
	return "## 技能创建任务\n\n" +
		"你需要为用户创建一个技能(Skill)。使用 write_file 工具在 skills/<skill-name>/SKILL.md 路径创建文件，严格遵循以下标准格式：\n\n" +
		"SKILL.md 格式：\n" +
		"---\n" +
		"name: <skill-name>\n" +
		"description: <简短描述>\n" +
		"metadata:\n" +
		"  openclaw:\n" +
		"    emoji: \"<emoji>\"\n" +
		"    requires:\n" +
		"      bins:\n" +
		"        - <依赖命令>\n" +
		"---\n\n" +
		"## 核心能力\n" +
		"- <列出核心功能点>\n\n" +
		"## 执行步骤\n" +
		"1. <第一步> {{变量名}}\n" +
		"2. <第二步>\n" +
		"3. <后续步骤>\n\n" +
		"## 输入格式\n" +
		"<需要的输入参数>\n\n" +
		"## 异常处理\n" +
		"- <异常情况的处理方式>\n\n" +
		"目录结构:\n" +
		"  skills/<skill-name>/SKILL.md (必需)\n" +
		"  skills/<skill-name>/scripts/ (可选, .sh/.py/.js)\n" +
		"  skills/<skill-name>/references/ (可选)\n\n" +
		"创建步骤:\n" +
		"1. 分析用户需求，确定技能名称和功能\n" +
		"2. 选择 emoji 图标\n" +
		"3. 使用 write_file 创建 SKILL.md\n" +
		"4. 如需脚本，创建 scripts/ 下的脚本文件\n" +
		"5. 完成后告知用户技能已创建，可通过 skill_use 调用\n"
}