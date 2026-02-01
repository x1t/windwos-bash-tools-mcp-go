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
	suite.server.mutex.Lock()
	defer suite.server.mutex.Unlock()

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

	_, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "bash_id is required")
	assert.Equal(suite.T(), "failed", output.Status)
}

// TestBashOutputHandler_TooLongBashID 测试过长的bash_id
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_TooLongBashID() {
	longID := strings.Repeat("a", 101)
	args := BashOutputArguments{
		BashID: longID,
	}

	_, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "bash_id is too long")
	assert.Equal(suite.T(), "failed", output.Status)
}

// TestBashOutputHandler_TaskNotFound 测试任务不存在
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_TaskNotFound() {
	args := BashOutputArguments{
		BashID: "nonexistent_task_id",
	}

	_, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "background task not found")
	assert.Equal(suite.T(), "not_found", output.Status)
}

// TestBashOutputHandler_ValidTask 测试有效任务
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_ValidTask() {
	taskID := "test_bash_12345"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo Hello World",
		Output:    "Hello World\n",
		Status:    "running",
		StartTime: time.Now(),
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := BashOutputArguments{
		BashID: taskID,
	}

	_, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "running", output.Status)
	assert.Contains(suite.T(), output.Output, "Hello World")

	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, taskID)
	suite.server.mutex.Unlock()
}

// TestBashOutputHandler_RunningTask 测试运行中的任务
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_RunningTask() {
	taskID := "test_running_task"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "ping -n 10 127.0.0.1",
		Output:    "Pinging 127.0.0.1...\n",
		Status:    "running",
		StartTime: time.Now(),
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := BashOutputArguments{
		BashID: taskID,
	}

	_, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "running", output.Status)

	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, taskID)
	suite.server.mutex.Unlock()
}

// TestBashOutputHandler_FailedTask 测试失败的任务
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_FailedTask() {
	taskID := "test_failed_task"
	exitCode := 1
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "nonexistent_command",
		Output:    "",
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

	_, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "failed", output.Status)
	assert.Equal(suite.T(), &exitCode, output.ExitCode)

	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, taskID)
	suite.server.mutex.Unlock()
}

// TestBashOutputHandler_WithFilter 测试带过滤器的输出
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_WithFilter() {
	taskID := "test_filter_task"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo -e 'INFO: Start\nDEBUG: Detail\nINFO: End'",
		Output:    "INFO: Start\nDEBUG: Detail\nINFO: End\n",
		Status:    "completed",
		StartTime: time.Now(),
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := BashOutputArguments{
		BashID: taskID,
		Filter: "^INFO:",
	}

	_, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "completed", output.Status)
	assert.Contains(suite.T(), output.Output, "INFO: Start")
	assert.Contains(suite.T(), output.Output, "INFO: End")
	assert.NotContains(suite.T(), output.Output, "DEBUG:")

	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, taskID)
	suite.server.mutex.Unlock()
}

// TestBashOutputHandler_InvalidFilter 测试无效过滤器
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_InvalidFilter() {
	// 创建一个任务用于测试
	taskID := "test_invalid_filter_task"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo test",
		Output:    "test output\n",
		Status:    "running",
		StartTime: time.Now(),
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := BashOutputArguments{
		BashID: taskID,
		Filter: "[invalid",
	}

	_, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "invalid filter pattern")
	assert.Equal(suite.T(), "failed", output.Status)

	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, taskID)
	suite.server.mutex.Unlock()
}

// TestBashOutputHandler_ConcurrentAccess 测试并发访问
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_ConcurrentAccess() {
	taskID := "test_concurrent_task"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo concurrent",
		Output:    "concurrent output\n",
		Status:    "running",
		StartTime: time.Now(),
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			args := BashOutputArguments{
				BashID: taskID,
			}

			_, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

			assert.NoError(suite.T(), err)
			assert.Equal(suite.T(), "running", output.Status)
		}()
	}

	wg.Wait()

	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, taskID)
	suite.server.mutex.Unlock()
}

// TestBashOutputHandler_TempFileReading 测试临时文件读取
func (suite *BashOutputHandlerTestSuite) TestBashOutputHandler_TempFileReading() {
	taskID := "test_temp_file_task"
	tempFile, err := os.CreateTemp("", "test_output_*.txt")
	require.NoError(suite.T(), err)
	defer os.Remove(tempFile.Name())

	tempFile.WriteString("Temp file content\n")
	tempFile.Close()

	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo test",
		Output:    "",
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

	_, output, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "running", output.Status)
	assert.Contains(suite.T(), output.Output, "Temp file content")

	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, taskID)
	suite.server.mutex.Unlock()
}

// 运行BashOutputHandler测试套件
func TestBashOutputHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(BashOutputHandlerTestSuite))
}
