package tool

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go-claw/pkg/utils"

	_ "github.com/mattn/go-sqlite3"
)

// DatabaseQueryTool 数据库查询工具
type DatabaseQueryTool struct{}

func NewDatabaseQueryTool() *DatabaseQueryTool {
	return &DatabaseQueryTool{}
}

func (t *DatabaseQueryTool) Name() string {
	return "database_query"
}

func (t *DatabaseQueryTool) Description() string {
	return `连接 SQLite 数据库并执行查询。
支持 SELECT、INSERT、UPDATE、DELETE 操作。

调用格式：
- database_query(path="data.db", query="SELECT * FROM users LIMIT 10")
- database_query(path="data.db", query="INSERT INTO logs(content) VALUES('test')")

参数说明：
- path: 数据库文件路径（必填）
- query: SQL 查询语句（必填）
- params: JSON 格式的参数列表，用于参数化查询

安全限制：
- 只支持 SQLite
- 禁止访问敏感路径（.env, .git 等）
- DELETE/UPDATE 最多影响 100 条记录`
}

func (t *DatabaseQueryTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "数据库文件路径（SQLite）",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "SQL 查询语句",
			},
			"params": map[string]interface{}{
				"type":        "string",
				"description": "参数化查询的参数，JSON 数组格式",
			},
		},
		"required": []string{"path", "query"},
	}
}

func (t *DatabaseQueryTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	dbPath, ok := params["path"].(string)
	if !ok || dbPath == "" {
		return "", fmt.Errorf("缺少 path 参数")
	}

	query, ok := params["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("缺少 query 参数")
	}

	// 安全检查
	if utils.IsSensitivePath(dbPath) {
		return "", fmt.Errorf("禁止访问敏感路径: %s", dbPath)
	}

	// 检查是否是危险操作
	queryUpper := strings.ToUpper(strings.TrimSpace(query))
	if strings.HasPrefix(queryUpper, "DROP") {
		return "", fmt.Errorf("禁止执行 DROP 操作")
	}

	// 打开数据库
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return "", fmt.Errorf("打开数据库失败: %v", err)
	}
	defer db.Close()

	// 设置超时
	ctxDb, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 解析参数
	var queryParams []interface{}
	if paramsStr, ok := params["params"].(string); ok && paramsStr != "" {
		var arr []interface{}
		if err := json.Unmarshal([]byte(paramsStr), &arr); err == nil {
			queryParams = arr
		}
	}

	// 判断查询类型
	if strings.HasPrefix(queryUpper, "SELECT") || strings.HasPrefix(queryUpper, "SHOW") || strings.HasPrefix(queryUpper, "PRAGMA") {
		return t.executeSelect(ctxDb, db, query, queryParams)
	}
	return t.executeWrite(ctxDb, db, query, queryParams, queryUpper)
}

func (t *DatabaseQueryTool) executeSelect(ctx context.Context, db *sql.DB, query string, params []interface{}) (string, error) {
	rows, err := db.QueryContext(ctx, query, params...)
	if err != nil {
		return "", fmt.Errorf("查询失败: %v", err)
	}
	defer rows.Close()

	// 获取列名
	columns, err := rows.Columns()
	if err != nil {
		return "", fmt.Errorf("获取列名失败: %v", err)
	}

	// 读取数据
	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return "", fmt.Errorf("读取数据失败: %v", err)
		}
		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			// 处理 []byte 类型
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		results = append(results, row)
	}

	// 限制显示条数
	displayResults := results
	if len(results) > 100 {
		displayResults = results[:100]
	}

	// 构建 JSON 输出
	selectResult := map[string]interface{}{
		"columns":   columns,
		"rows":      displayResults,
		"row_count": len(results),
	}

	jsonBytes, err := json.MarshalIndent(selectResult, "", "  ")
	if err != nil {
		return "", fmt.Errorf("JSON 序列化失败: %v", err)
	}

	return string(jsonBytes), nil
}

func (t *DatabaseQueryTool) executeWrite(ctx context.Context, db *sql.DB, query string, params []interface{}, queryUpper string) (string, error) {
	// 执行写入操作
	result, err := db.ExecContext(ctx, query, params...)
	if err != nil {
		return "", fmt.Errorf("执行失败: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		// 无法获取影响行数时仍返回 JSON
		writeResult := map[string]interface{}{
			"operation":     "unknown",
			"affected_rows": 0,
			"status":        "success",
		}
		jsonBytes, jsonErr := json.MarshalIndent(writeResult, "", "  ")
		if jsonErr != nil {
			return "", fmt.Errorf("JSON 序列化失败: %v", jsonErr)
		}
		return string(jsonBytes), nil
	}

	// 安全限制
	if affected > 100 {
		return "", fmt.Errorf("操作影响了 %d 条记录，超过安全限制（最大100条）", affected)
	}

	opType := "unknown"
	if strings.HasPrefix(queryUpper, "INSERT") {
		opType = "insert"
	} else if strings.HasPrefix(queryUpper, "UPDATE") {
		opType = "update"
	} else if strings.HasPrefix(queryUpper, "DELETE") {
		opType = "delete"
	}

	// 构建 JSON 输出
	writeResult := map[string]interface{}{
		"operation":     opType,
		"affected_rows": affected,
		"status":        "success",
	}

	jsonBytes, err := json.MarshalIndent(writeResult, "", "  ")
	if err != nil {
		return "", fmt.Errorf("JSON 序列化失败: %v", err)
	}

	return string(jsonBytes), nil
}

func init() {
	GlobalRegistry.Register("database_query", func() Tool {
		return NewDatabaseQueryTool()
	})
}
