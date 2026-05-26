---
name: weather-query
description: 查询指定城市的实时天气信息和天气预报
metadata:
  openclaw:
    emoji: "🌤️"
    requires:
      bins:
        - curl
---

## 核心能力

- 查询任意城市的当前天气状况（温度、湿度、风力、天气状况）
- 获取未来几天的天气预报
- 支持中文和英文城市名

## 执行步骤

1. 确认用户要查询的城市名称 {{city}}
2. 如果有天气 API 工具可用，直接调用 weather 工具查询
3. 如果没有天气 API 工具，使用 curl 调用 wttr.in 服务: `curl -s wttr.in/{{city}}?format=3`

## 输出格式

返回城市的天气信息，包括:
- 当前温度和天气状况
- 体感温度
- 风速和风向
- 湿度

## 异常处理

- 如果城市名无法识别，提示用户确认城市名
- 如果 API 调用失败，返回错误信息并建议稍后重试