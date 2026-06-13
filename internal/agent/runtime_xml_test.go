package agent

import (
	"encoding/json"
	"testing"
)

func TestExtractXMLToolCalls_InvokeFormat(t *testing.T) {
	// 来自实际日志的 content
	content := "\n\n<tool_call>\n<invoke name=\"execute_command\">\n<parameter name=\"command\">chmod 777 /app/AGENTS.md</parameter>\n</invoke>\n</minimax:tool_call>"

	toolCalls, cleaned := extractXMLToolCallsWithCleanup(content)

	if len(toolCalls) == 0 {
		t.Fatalf("期望提取到 1 个工具调用，实际提取到 0 个")
	}

	if toolCalls[0].Function.Name != "execute_command" {
		t.Errorf("期望工具名为 execute_command，实际为 %s", toolCalls[0].Function.Name)
	}

	// 验证参数
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("参数 JSON 解析失败: %v", err)
	}

	if args["command"] != "chmod 777 /app/AGENTS.md" {
		t.Errorf("期望 command 参数为 'chmod 777 /app/AGENTS.md'，实际为 %v", args["command"])
	}

	t.Logf("提取成功: tool=%s, args=%s, cleaned=%q", toolCalls[0].Function.Name, toolCalls[0].Function.Arguments, cleaned)
}

func TestExtractXMLToolCalls_ToolUseFormat(t *testing.T) {
	// Anthropic Claude 风格
	content := `<tool_use name="read_file">
<input>{"path": "/etc/passwd"}</input>
</tool_use>`

	toolCalls, _ := extractXMLToolCallsWithCleanup(content)

	if len(toolCalls) == 0 {
		t.Fatalf("期望提取到 1 个工具调用，实际提取到 0 个")
	}

	if toolCalls[0].Function.Name != "read_file" {
		t.Errorf("期望工具名为 read_file，实际为 %s", toolCalls[0].Function.Name)
	}

	t.Logf("提取成功: tool=%s, args=%s", toolCalls[0].Function.Name, toolCalls[0].Function.Arguments)
}

func TestExtractXMLToolCalls_MultipleInvoke(t *testing.T) {
	// 多个工具调用
	content := `<invoke name="read_file">
<parameter name="path">test.txt</parameter>
</invoke>
<invoke name="write_file">
<parameter name="path">output.txt</parameter>
<parameter name="content">hello</parameter>
</invoke>`

	toolCalls, _ := extractXMLToolCallsWithCleanup(content)

	if len(toolCalls) != 2 {
		t.Fatalf("期望提取到 2 个工具调用，实际提取到 %d 个", len(toolCalls))
	}

	t.Logf("提取成功: count=%d, tools=%s,%s", len(toolCalls), toolCalls[0].Function.Name, toolCalls[1].Function.Name)
}

func TestExtractXMLToolCalls_NoMatch(t *testing.T) {
	// 普通 text，无 XML 工具调用
	content := "这是一条普通消息，没有工具调用"

	toolCalls, cleaned := extractXMLToolCallsWithCleanup(content)

	if len(toolCalls) != 0 {
		t.Errorf("期望不提取任何工具调用，实际提取到 %d 个", len(toolCalls))
	}

	if cleaned != content {
		t.Errorf("期望 content 不变，实际 cleaned=%q", cleaned)
	}
}