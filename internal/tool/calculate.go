package tool

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// CalculateTool 数学计算工具
type CalculateTool struct{}

func NewCalculateTool() *CalculateTool {
	return &CalculateTool{}
}

func (t *CalculateTool) Name() string {
	return "calculate"
}

func (t *CalculateTool) Description() string {
	return `计算数学表达式，支持四则运算、幂运算、开方、三角函数、对数等。
解决 AI 计算不准确的问题，精确计算交给工具执行。

调用格式：
- calculate(expression="2^10")  # 2的10次方 = 1024
- calculate(expression="sqrt(16)")  # 开方 = 4
- calculate(expression="sin(3.14159/2)")  # 正弦
- calculate(expression="log(100, 10)")  # 对数
- calculate(expression="(3.5 + 2.1) * 4 / 2")  # 四则运算

支持函数：
- sqrt(x): 平方根
- abs(x): 绝对值
- pow(x, y): x的y次方
- sin(x), cos(x), tan(x): 三角函数（弧度）
- log(x, base): 对数，默认自然对数
- floor(x), ceil(x), round(x): 取整`
}

func (t *CalculateTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"expression": map[string]interface{}{
				"type":        "string",
				"description": "数学表达式",
			},
		},
		"required": []string{"expression"},
	}
}

func (t *CalculateTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	expr, ok := params["expression"].(string)
	if !ok || expr == "" {
		return "", fmt.Errorf("缺少 expression 参数")
	}

	result, err := evalExpression(expr)
	if err != nil {
		return "", err
	}

	// 格式化结果
	resultStr := formatResult(result)
	return fmt.Sprintf("计算结果: %s = %s", expr, resultStr), nil
}

func evalExpression(expr string) (float64, error) {
	expr = strings.TrimSpace(expr)
	expr = strings.ReplaceAll(expr, " ", "")

	// 处理函数调用
	if strings.Contains(expr, "(") {
		return evalFunction(expr)
	}

	// 处理运算符表达式
	return evalArithmetic(expr)
}

func evalFunction(expr string) (float64, error) {
	// 匹配 func(args) 模式
	re := regexp.MustCompile(`^(\w+)\((.+)\)$`)
	matches := re.FindStringSubmatch(expr)
	if len(matches) < 3 {
		return 0, fmt.Errorf("无效的函数表达式: %s", expr)
	}

	funcName := matches[1]
	argsStr := matches[2]

	// 解析参数
	args := parseArgs(argsStr)
	if len(args) == 0 {
		return 0, fmt.Errorf("函数参数为空")
	}

	switch funcName {
	case "sqrt":
		return math.Sqrt(args[0]), nil
	case "abs":
		return math.Abs(args[0]), nil
	case "pow":
		if len(args) < 2 {
			return 0, fmt.Errorf("pow 需要两个参数")
		}
		return math.Pow(args[0], args[1]), nil
	case "sin":
		return math.Sin(args[0]), nil
	case "cos":
		return math.Cos(args[0]), nil
	case "tan":
		return math.Tan(args[0]), nil
	case "log":
		if len(args) >= 2 {
			return math.Log(args[0]) / math.Log(args[1]), nil
		}
		return math.Log(args[0]), nil
	case "ln":
		return math.Log(args[0]), nil
	case "log10":
		return math.Log10(args[0]), nil
	case "exp":
		return math.Exp(args[0]), nil
	case "floor":
		return math.Floor(args[0]), nil
	case "ceil":
		return math.Ceil(args[0]), nil
	case "round":
		return math.Round(args[0]), nil
	default:
		return 0, fmt.Errorf("未知函数: %s", funcName)
	}
}

func parseArgs(s string) []float64 {
	var args []float64
	// 简单分割，不支持嵌套函数
	parts := strings.Split(s, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		// 先尝试计算表达式
		val, err := evalArithmetic(p)
		if err != nil {
			// 直接解析数字
			val, err = strconv.ParseFloat(p, 64)
			if err != nil {
				continue
			}
		}
		args = append(args, val)
	}
	return args
}

func evalArithmetic(expr string) (float64, error) {
	// 处理幂运算 ^
	if strings.Contains(expr, "^") {
		parts := strings.SplitN(expr, "^", 2)
		if len(parts) == 2 {
			base, err := evalArithmetic(parts[0])
			if err != nil {
				return 0, err
			}
			exp, err := evalArithmetic(parts[1])
			if err != nil {
				return 0, err
			}
			return math.Pow(base, exp), nil
		}
	}

	// 处理括号
	for strings.Contains(expr, "(") {
		// 找最内层括号
		re := regexp.MustCompile(`\(([^()]+)\)`)
		match := re.FindStringSubmatch(expr)
		if len(match) < 2 {
			break
		}
		inner := match[1]
		val, err := evalSimple(inner)
		if err != nil {
			return 0, err
		}
		expr = strings.Replace(expr, match[0], fmt.Sprintf("%.15g", val), 1)
	}

	return evalSimple(expr)
}

func evalSimple(expr string) (float64, error) {
	// 处理乘除
	for {
		// 找 * 或 /
		idxMul := strings.Index(expr, "*")
		idxDiv := strings.Index(expr, "/")
		idx := -1
		op := ""
		if idxMul >= 0 && (idxDiv < 0 || idxMul < idxDiv) {
			idx = idxMul
			op = "*"
		} else if idxDiv >= 0 {
			idx = idxDiv
			op = "/"
		}
		if idx < 0 {
			break
		}

		// 提取左右操作数
		left, leftEnd := extractNumberLeft(expr, idx)
		right, rightStart := extractNumberRight(expr, idx)

		var result float64
		if op == "*" {
			result = left * right
		} else {
			if right == 0 {
				return 0, fmt.Errorf("除数不能为0")
			}
			result = left / right
		}

		expr = expr[:leftEnd] + fmt.Sprintf("%.15g", result) + expr[rightStart:]
	}

	// 处理加减
	for {
		// 找 + 或 -（跳过开头的负号）
		idxPlus := strings.Index(expr[1:], "+")
		if idxPlus >= 0 {
			idxPlus++
		}
		idxMinus := strings.Index(expr[1:], "-")
		if idxMinus >= 0 {
			idxMinus++
		}
		idx := -1
		op := ""
		if idxPlus >= 0 && (idxMinus < 0 || idxPlus < idxMinus) {
			idx = idxPlus
			op = "+"
		} else if idxMinus >= 0 {
			idx = idxMinus
			op = "-"
		}
		if idx < 0 {
			break
		}

		left, leftEnd := extractNumberLeft(expr, idx)
		right, rightStart := extractNumberRight(expr, idx)

		var result float64
		if op == "+" {
			result = left + right
		} else {
			result = left - right
		}

		expr = expr[:leftEnd] + fmt.Sprintf("%.15g", result) + expr[rightStart:]
	}

	return strconv.ParseFloat(expr, 64)
}

func extractNumberLeft(expr string, opIdx int) (float64, int) {
	// 从 opIdx 向左找数字
	i := opIdx - 1
	for i >= 0 && (expr[i] >= '0' && expr[i] <= '9' || expr[i] == '.' || expr[i] == '-' || expr[i] == 'e' || expr[i] == 'E') {
		i--
	}
	num, _ := strconv.ParseFloat(expr[i+1:opIdx], 64)
	return num, i + 1
}

func extractNumberRight(expr string, opIdx int) (float64, int) {
	// 从 opIdx+1 向右找数字
	i := opIdx + 1
	if i < len(expr) && expr[i] == '-' {
		i++
	}
	for i < len(expr) && (expr[i] >= '0' && expr[i] <= '9' || expr[i] == '.' || expr[i] == 'e' || expr[i] == 'E') {
		i++
	}
	num, _ := strconv.ParseFloat(expr[opIdx+1:i], 64)
	return num, i
}

func formatResult(val float64) string {
	// 如果是整数，显示整数形式
	if val == math.Floor(val) && math.Abs(val) < 1e15 {
		return fmt.Sprintf("%.0f", val)
	}
	// 否则保留合理精度
	str := fmt.Sprintf("%.10f", val)
	// 去除末尾的0
	str = strings.TrimRight(str, "0")
	str = strings.TrimRight(str, ".")
	return str
}

func init() {
	GlobalRegistry.Register("calculate", func() Tool {
		return NewCalculateTool()
	})
}