package main

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// BashOutputHandlerTestSuite Bash输出处理器测试套件
type BashOutputHandlerTestSuite struct {
	suite.Suite
	server *MCPServer
}

// SetupSuite 测试套件初始化
func (suite *BashOutputHandlerTestSuite) SetupSuite() {
	suite.server = NewMCPServer()
}

// TearDownSuite 测试套件清理
func (suite *BashOutputHandlerTestSuite) TearDownSuite() {
	// 清理所有后台任务
	suite.server.mutex.Lock()
	defer suite.server.mutex.Unlock()
	
	// 清理临时文件
	for _, task := range suite.server.backgroundTasks {
		if task.TempFile != "" {
			os.Remove(task.TempFile)
		}
	}
	suite.server.backgroundTasks = make(map[string]*BackgroundTask)
}

// TestBashOutputHandler_EmptyBashID 测试空bash_id错误
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_EmptyBashID() {
	args := BashOutputArguments{
		BashID: "",
	}

	// 根据官方MCP标准，工具错误返回nil + 结构化输出
	result, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "bash_id参数是必需的")
	
	// 工具错误时result为nil
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), "failed", output.Status)
}

// TestBashOutputHandler_TooLongBashID 测试过长的bash_id
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_TooLongBashID() {
	longID := strings.Repeat("a", 101) // 101个字符
	args := BashOutputArguments{
		BashID: longID,
	}

	result, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "bash_id过长")
	
	// 工具错误时result为nil
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), "failed", output.Status)
}

// TestBashOutputHandler_TaskNotFound 测试任务不存在
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_TaskNotFound() {
	args := BashOutputArguments{
		BashID: "nonexistent_task_id",
	}

	result, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "未找到后台任务")
	
	// 工具错误时result为nil
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), "not_found", output.Status)
}

// TestBashOutputHandler_ValidTask 测试有效任务
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_ValidTask() {
	// 创建一个测试任务
	taskID := "test_bash_12345"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo Hello World",
		Output:    "Hello World\n",
		Status:    "completed",
		StartTime: time.Now(),
		ExitCode:  &[]int{0}[0], // 获取指向0的指针
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := BashOutputArguments{
		BashID: taskID,
	}

	// 根据官方MCP标准，成功操作返回nil
	result, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	// 成功操作返回nil
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), "Hello World\n", output.Output)
	assert.Equal(suite.T(), "completed", output.Status)
	assert.NotNil(suite.T(), output.ExitCode)
	assert.Equal(suite.T(), 0, *output.ExitCode)

	// 清理
	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, taskID)
	suite.server.mutex.Unlock()
}

// TestBashOutputHandler_RunningTask 测试运行中的任务
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_RunningTask() {
	// 创建一个运行中的任务
	taskID := "test_running_12345"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "sleep 10",
		Output:    "Starting sleep...\n",
		Status:    "running",
		StartTime: time.Now(),
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := BashOutputArguments{
		BashID: taskID,
	}

	// 根据官方MCP标准，成功操作返回nil
	result, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	// 成功操作返回nil
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), "Starting sleep...\n", output.Output)
	assert.Equal(suite.T(), "running", output.Status)
	assert.Nil(suite.T(), output.ExitCode) // 运行中的任务没有ExitCode

	// 清理
	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, taskID)
	suite.server.mutex.Unlock()
}

// TestBashOutputHandler_FailedTask 测试失败的任务
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_FailedTask() {
	// 创建一个失败的任务
	taskID := "test_failed_12345"
	exitCode := 1
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "invalid_command",
		Output:    "Error: command not found\n",
		Status:    "failed",
		StartTime: time.Now(),
		Error:     "command not found",
		ExitCode:  &exitCode,
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := BashOutputArguments{
		BashID: taskID,
	}

	// 根据官方MCP标准，成功操作返回nil
	result, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	// 成功操作返回nil
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), "Error: command not found\n", output.Output)
	assert.Equal(suite.T(), "failed", output.Status)
	assert.NotNil(suite.T(), output.ExitCode)
	assert.Equal(suite.T(), 1, *output.ExitCode)

	// 清理
	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, taskID)
	suite.server.mutex.Unlock()
}

// TestBashOutputHandler_WithFilter 测试输出过滤
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_WithFilter() {
	// 创建一个包含多行输出的任务
	taskID := "test_filter_12345"
	output := `Line 1: INFO Starting process
Line 2: ERROR Something went wrong
Line 3: INFO Process completed
Line 4: DEBUG Debug information
Line 5: ERROR Another error`

	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo multiline",
		Output:    output,
		Status:    "completed",
		StartTime: time.Now(),
		ExitCode:  &[]int{0}[0],
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	tests := []struct {
		name           string
		filter         string
		expectedLines  []string
	}{
		{
			name:          "过滤ERROR行",
			filter:        "ERROR",
			expectedLines: []string{"Line 2: ERROR Something went wrong", "Line 5: ERROR Another error"},
		},
		{
			name:          "过滤INFO行",
			filter:        "INFO",
			expectedLines: []string{"Line 1: INFO Starting process", "Line 3: INFO Process completed"},
		},
		{
			name:           "数字过滤",
			filter:         "Line [0-9]+:",
			expectedLines:  []string{"Line 1: INFO Starting process", "Line 2: ERROR Something went wrong", "Line 3: INFO Process completed", "Line 4: DEBUG Debug information", "Line 5: ERROR Another error"},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			args := BashOutputArguments{
				BashID: taskID,
				Filter: tt.filter,
			}

			// 根据官方MCP标准，成功操作返回nil
			result, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

			require.NoError(suite.T(), err)
			// 成功操作返回nil
			assert.Nil(suite.T(), result)

			// 验证过滤结果
			lines := strings.Split(output.Output, "\n")
			// 移除最后一个空行（如果存在）
			if lines[len(lines)-1] == "" {
				lines = lines[:len(lines)-1]
			}

			assert.Len(suite.T(), lines, len(tt.expectedLines))
			for i, expectedLine := range tt.expectedLines {
				assert.Equal(suite.T(), expectedLine, lines[i])
			}
		})
	}

	// 清理
	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, taskID)
	suite.server.mutex.Unlock()
}

// TestBashOutputHandler_InvalidFilter 测试无效的正则表达式
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_InvalidFilter() {
	// 创建一个测试任务
	taskID := "test_invalid_filter_12345"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo test",
		Output:    "test output",
		Status:    "completed",
		StartTime: time.Now(),
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := BashOutputArguments{
		BashID: taskID,
		Filter: "[invalid regex", // 无效的正则表达式
	}

	result, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "无效的过滤模式")
	
	// 工具错误时result为nil
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), "failed", output.Status)

	// 清理
	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, taskID)
	suite.server.mutex.Unlock()
}

// TestBashOutputHandler_TempFileReading 测试临时文件读取
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_TempFileReading() {
	// 创建临时文件
	tempFile, err := os.CreateTemp("", "bash_test_*.txt")
	require.NoError(suite.T(), err)
	defer os.Remove(tempFile.Name())

	// 写入测试内容
	testContent := "Content from temp file\nLine 2\nLine 3"
	_, err = tempFile.WriteString(testContent)
	require.NoError(suite.T(), err)
	tempFile.Close()

	// 创建使用临时文件的任务
	taskID := "test_tempfile_12345"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo with temp file",
		Output:    "Old output",
		Status:    "running",
		StartTime: time.Now(),
		TempFile:  tempFile.Name(),
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := BashOutputArguments{
		BashID: taskID,
	}

	// 根据官方MCP标准，成功操作返回nil
	result, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	// 成功操作返回nil
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), testContent, output.Output)
	assert.Equal(suite.T(), "running", output.Status)

	// 清理
	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, taskID)
	suite.server.mutex.Unlock()
}

// TestBashOutputHandler_ConcurrentAccess 测试并发访问
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_ConcurrentAccess() {
	// 创建多个任务
	const numTasks = 5
	taskIDs := make([]string, numTasks)
	
	for i := 0; i < numTasks; i++ {
		taskID := "test_concurrent_" + string(rune('A'+i))
		taskIDs[i] = taskID
		
		task := &BackgroundTask{
			ID:        taskID,
			Command:   "echo task " + string(rune('A'+i)),
			Output:    "Output " + string(rune('A'+i)) + "\n",
			Status:    "completed",
			StartTime: time.Now(),
			ExitCode:  &[]int{0}[0],
		}
		
		suite.server.mutex.Lock()
		suite.server.backgroundTasks[taskID] = task
		suite.server.mutex.Unlock()
	}

	// 并发访问所有任务
	var wg sync.WaitGroup
	for _, taskID := range taskIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			
			args := BashOutputArguments{
				BashID: id,
			}

			// 根据官方MCP标准，成功操作返回nil
			result, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

			assert.NoError(suite.T(), err)
			// 成功操作返回nil
			assert.Nil(suite.T(), result)
			assert.NotEmpty(suite.T(), output.Output)
			assert.Equal(suite.T(), "completed", output.Status)
		}(taskID)
	}

	wg.Wait()

	// 清理
	suite.server.mutex.Lock()
	for _, taskID := range taskIDs {
		delete(suite.server.backgroundTasks, taskID)
	}
	suite.server.mutex.Unlock()
}

// 运行BashOutputHandler测试套件
func TestBashOutputHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(BashOutputHandlerTestSuite))
}

// 基准测试
func BenchmarkBashOutputHandler_ValidTask(b *testing.B) {
	server := NewMCPServer()
	
	// 创建测试任务
	taskID := "benchmark_task"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo benchmark",
		Output:    "benchmark output",
		Status:    "completed",
		StartTime: time.Now(),
		ExitCode:  &[]int{0}[0],
	}
	
	server.mutex.Lock()
	server.backgroundTasks[taskID] = task
	server.mutex.Unlock()

	args := BashOutputArguments{
		BashID: taskID,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)
		if err != nil {
			b.Fatal(err)
		}
	}

	// 清理
	server.mutex.Lock()
	delete(server.backgroundTasks, taskID)
	server.mutex.Unlock()
}