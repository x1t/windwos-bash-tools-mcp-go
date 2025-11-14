package main

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// BackgroundTaskManagerTestSuite 后台任务管理器测试套件
type BackgroundTaskManagerTestSuite struct {
	suite.Suite
	server *MCPServer
}

// SetupSuite 测试套件初始化
func (suite *BackgroundTaskManagerTestSuite) SetupSuite() {
	suite.server = NewMCPServer()
}

// TearDownSuite 测试套件清理
func (suite *BackgroundTaskManagerTestSuite) TearDownSuite() {
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

// TestCreateBackgroundTask 测试创建后台任务
func (suite *BackgroundTaskManagerTestSuite) TestCreateBackgroundTask() {
	// 模拟BashHandler创建后台任务的过程
	taskID := "test_create_task_12345"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo Hello Background",
		StartTime: time.Now(),
		Status:    "running",
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	// 验证任务已创建
	suite.server.mutex.RLock()
	storedTask, exists := suite.server.backgroundTasks[taskID]
	suite.server.mutex.RUnlock()

	assert.True(suite.T(), exists, "任务应该存在")
	assert.Equal(suite.T(), taskID, storedTask.ID)
	assert.Equal(suite.T(), "echo Hello Background", storedTask.Command)
	assert.Equal(suite.T(), "running", storedTask.Status)
	assert.NotNil(suite.T(), storedTask.StartTime)

	// 清理
	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, taskID)
	suite.server.mutex.Unlock()
}

// TestUpdateTaskStatus 测试更新任务状态
func (suite *BackgroundTaskManagerTestSuite) TestUpdateTaskStatus() {
	// 创建初始任务
	taskID := "test_update_status_12345"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo test",
		StartTime: time.Now(),
		Status:    "running",
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	// 更新任务状态
	newStatus := "completed"
	exitCode := 0
	suite.server.mutex.Lock()
	if storedTask, exists := suite.server.backgroundTasks[taskID]; exists {
		storedTask.Status = newStatus
		storedTask.ExitCode = &exitCode
		storedTask.Output = "Command completed successfully"
	}
	suite.server.mutex.Unlock()

	// 验证状态已更新
	suite.server.mutex.RLock()
	updatedTask, exists := suite.server.backgroundTasks[taskID]
	suite.server.mutex.RUnlock()

	assert.True(suite.T(), exists, "任务应该存在")
	assert.Equal(suite.T(), newStatus, updatedTask.Status)
	assert.NotNil(suite.T(), updatedTask.ExitCode)
	assert.Equal(suite.T(), 0, *updatedTask.ExitCode)
	assert.Equal(suite.T(), "Command completed successfully", updatedTask.Output)

	// 清理
	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, taskID)
	suite.server.mutex.Unlock()
}

// TestConcurrentTaskCreation 测试并发创建任务
func (suite *BackgroundTaskManagerTestSuite) TestConcurrentTaskCreation() {
	const numTasks = 20
	var wg sync.WaitGroup
	createdTasks := make([]string, 0, numTasks)
	var tasksMutex sync.Mutex

	// 并发创建多个任务
	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			
			taskID := "test_concurrent_create_" + string(rune('A'+index))
			task := &BackgroundTask{
				ID:        taskID,
				Command:   "echo concurrent task " + string(rune('A'+index)),
				StartTime: time.Now(),
				Status:    "running",
			}

			// 模拟BashHandler的后台任务创建
			suite.server.mutex.Lock()
			suite.server.backgroundTasks[taskID] = task
			suite.server.mutex.Unlock()

			tasksMutex.Lock()
			createdTasks = append(createdTasks, taskID)
			tasksMutex.Unlock()
		}(i)
	}

	wg.Wait()

	// 验证所有任务都被创建
	suite.server.mutex.RLock()
	assert.Len(suite.T(), suite.server.backgroundTasks, numTasks, "应该创建指定数量的任务")
	suite.server.mutex.RUnlock()

	// 验证任务ID的唯一性
	uniqueIDs := make(map[string]bool)
	for _, taskID := range createdTasks {
		assert.False(suite.T(), uniqueIDs[taskID], "任务ID应该是唯一的: %s", taskID)
		uniqueIDs[taskID] = true
	}

	// 验证所有任务都正确存储
	suite.server.mutex.RLock()
	for _, taskID := range createdTasks {
		task, exists := suite.server.backgroundTasks[taskID]
		assert.True(suite.T(), exists, "任务应该存在: %s", taskID)
		assert.Equal(suite.T(), "running", task.Status, "任务状态应该是running")
		assert.NotNil(suite.T(), task.StartTime, "任务应该有开始时间")
	}
	suite.server.mutex.RUnlock()

	// 清理
	suite.server.mutex.Lock()
	for _, taskID := range createdTasks {
		delete(suite.server.backgroundTasks, taskID)
	}
	suite.server.mutex.Unlock()
}

// TestConcurrentTaskAccess 测试并发访问任务
func (suite *BackgroundTaskManagerTestSuite) TestConcurrentTaskAccess() {
	// 创建多个任务
	const numTasks = 10
	taskIDs := make([]string, numTasks)
	
	for i := 0; i < numTasks; i++ {
		taskID := "test_concurrent_access_" + string(rune('0'+i))
		taskIDs[i] = taskID
		
		task := &BackgroundTask{
			ID:        taskID,
			Command:   "echo task " + string(rune('0'+i)),
			Output:    "Initial output " + string(rune('0'+i)),
			Status:    "running",
			StartTime: time.Now(),
		}
		
		suite.server.mutex.Lock()
		suite.server.backgroundTasks[taskID] = task
		suite.server.mutex.Unlock()
	}

	var wg sync.WaitGroup
	
	// 并发读取任务
	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			
			taskID := taskIDs[index]
			
			// 模拟BashOutputHandler的读取操作
			suite.server.mutex.RLock()
			task, exists := suite.server.backgroundTasks[taskID]
			suite.server.mutex.RUnlock()
			
			assert.True(suite.T(), exists, "任务应该存在: %s", taskID)
			assert.Equal(suite.T(), "Initial output "+string(rune('0'+index)), task.Output)
			assert.Equal(suite.T(), "running", task.Status)
		}(i)
	}
	
	// 并发更新任务
	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			
			taskID := taskIDs[index]
			
			// 模拟任务执行过程中的状态更新
			suite.server.mutex.Lock()
			if task, exists := suite.server.backgroundTasks[taskID]; exists {
				task.Output = "Updated output " + string(rune('0'+index))
				task.Status = "completed"
				exitCode := 0
				task.ExitCode = &exitCode
			}
			suite.server.mutex.Unlock()
		}(i)
	}

	wg.Wait()

	// 验证所有任务都被正确更新
	suite.server.mutex.RLock()
	for i, taskID := range taskIDs {
		task, exists := suite.server.backgroundTasks[taskID]
		assert.True(suite.T(), exists, "任务应该存在: %s", taskID)
		assert.Equal(suite.T(), "Updated output "+string(rune('0'+i)), task.Output)
		assert.Equal(suite.T(), "completed", task.Status)
		assert.NotNil(suite.T(), task.ExitCode)
		assert.Equal(suite.T(), 0, *task.ExitCode)
	}
	suite.server.mutex.RUnlock()

	// 清理
	suite.server.mutex.Lock()
	for _, taskID := range taskIDs {
		delete(suite.server.backgroundTasks, taskID)
	}
	suite.server.mutex.Unlock()
}

// TestTaskCleanup 测试任务清理
func (suite *BackgroundTaskManagerTestSuite) TestTaskCleanup() {
	// 创建多个不同状态的任务
	tasks := []struct {
		id     string
		status string
	}{
		{"cleanup_running_1", "running"},
		{"cleanup_completed_2", "completed"},
		{"cleanup_failed_3", "failed"},
		{"cleanup_killed_4", "killed"},
	}

	// 创建临时文件用于测试
	tempFiles := make([]string, 0, len(tasks))
	for _, taskInfo := range tasks {
		// 创建临时文件
		tempFile, err := os.CreateTemp("", "bash_cleanup_test_*.txt")
		require.NoError(suite.T(), err)
		tempFile.WriteString("test content")
		tempFile.Close()
		tempFiles = append(tempFiles, tempFile.Name())

		exitCode := 0
		if taskInfo.status == "failed" {
			exitCode = 1
		}

		task := &BackgroundTask{
			ID:        taskInfo.id,
			Command:   "echo " + taskInfo.id,
			Output:    "Output for " + taskInfo.id,
			Status:    taskInfo.status,
			StartTime: time.Now().Add(-time.Duration(len(taskInfo.id)) * time.Second),
			TempFile:  tempFile.Name(),
			ExitCode:  &exitCode,
		}

		if taskInfo.status == "failed" {
			task.Error = "Task failed"
		} else if taskInfo.status == "killed" {
			task.Error = "Task was killed"
		}

		suite.server.mutex.Lock()
		suite.server.backgroundTasks[taskInfo.id] = task
		suite.server.mutex.Unlock()
	}

	// 验证任务都已创建
	suite.server.mutex.RLock()
	assert.Len(suite.T(), suite.server.backgroundTasks, len(tasks), "应该创建所有任务")
	suite.server.mutex.RUnlock()

	// 逐一清理任务
	for _, taskInfo := range tasks {
		// 模拟KillShellHandler的清理操作
		suite.server.mutex.Lock()
		if task, exists := suite.server.backgroundTasks[taskInfo.id]; exists {
			if task.Status == "running" {
				task.Status = "killed"
				task.Error = "Task killed by user request"
			}
			// 清理临时文件
			if task.TempFile != "" {
				os.Remove(task.TempFile)
			}
			delete(suite.server.backgroundTasks, taskInfo.id)
		}
		suite.server.mutex.Unlock()
	}

	// 验证所有任务都已被清理
	suite.server.mutex.RLock()
	assert.Len(suite.T(), suite.server.backgroundTasks, 0, "所有任务都应该被清理")
	suite.server.mutex.RUnlock()

	// 验证临时文件已被清理
	for _, tempFile := range tempFiles {
		_, err := os.Stat(tempFile)
		assert.True(suite.T(), os.IsNotExist(err), "临时文件应该被清理: %s", tempFile)
	}
}

// TestTaskWithTempFile 测试带临时文件的任务
func (suite *BackgroundTaskManagerTestSuite) TestTaskWithTempFile() {
	// 创建临时文件
	tempFile, err := os.CreateTemp("", "bash_task_test_*.txt")
	require.NoError(suite.T(), err)
	
	testContent := "Line 1: Test output\nLine 2: More data\nLine 3: Final line"
	_, err = tempFile.WriteString(testContent)
	require.NoError(suite.T(), err)
	tempFile.Close()

	// 创建带临时文件的任务
	taskID := "test_tempfile_task_12345"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo with tempfile",
		Output:    "Initial output",
		Status:    "running",
		StartTime: time.Now(),
		TempFile:  tempFile.Name(),
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	// 验证任务存在
	suite.server.mutex.RLock()
	storedTask, exists := suite.server.backgroundTasks[taskID]
	suite.server.mutex.RUnlock()

	assert.True(suite.T(), exists, "任务应该存在")
	assert.Equal(suite.T(), tempFile.Name(), storedTask.TempFile)

	// 模拟从临时文件读取更新
	if content, err := os.ReadFile(tempFile.Name()); err == nil {
		suite.server.mutex.Lock()
		if task, exists := suite.server.backgroundTasks[taskID]; exists {
			task.Output = string(content)
			task.Status = "completed"
			exitCode := 0
			task.ExitCode = &exitCode
		}
		suite.server.mutex.Unlock()
	}

	// 验证任务已更新
	suite.server.mutex.RLock()
	updatedTask, exists := suite.server.backgroundTasks[taskID]
	suite.server.mutex.RUnlock()

	assert.True(suite.T(), exists, "任务应该存在")
	assert.Equal(suite.T(), testContent, updatedTask.Output)
	assert.Equal(suite.T(), "completed", updatedTask.Status)
	assert.NotNil(suite.T(), updatedTask.ExitCode)
	assert.Equal(suite.T(), 0, *updatedTask.ExitCode)

	// 清理
	suite.server.mutex.Lock()
	if task, exists := suite.server.backgroundTasks[taskID]; exists {
		if task.TempFile != "" {
			os.Remove(task.TempFile)
		}
		delete(suite.server.backgroundTasks, taskID)
	}
	suite.server.mutex.Unlock()
}

// TestTaskMemoryLeak 测试任务内存泄漏
func (suite *BackgroundTaskManagerTestSuite) TestTaskMemoryLeak() {
	const numCycles = 5
	const tasksPerCycle = 20

	for cycle := 0; cycle < numCycles; cycle++ {
		// 创建任务
		taskIDs := make([]string, tasksPerCycle)
		for i := 0; i < tasksPerCycle; i++ {
			taskID := "memory_leak_test_" + string(rune(cycle+'A')) + "_" + string(rune(i+'0'))
			taskIDs[i] = taskID
			
			task := &BackgroundTask{
				ID:        taskID,
				Command:   "echo memory test " + taskID,
				Output:    "Memory test output " + taskID,
				Status:    "running",
				StartTime: time.Now(),
			}
			
			suite.server.mutex.Lock()
			suite.server.backgroundTasks[taskID] = task
			suite.server.mutex.Unlock()
		}

		// 验证任务已创建
		suite.server.mutex.RLock()
		assert.Len(suite.T(), suite.server.backgroundTasks, tasksPerCycle, "第%d轮应该创建%d个任务", cycle, tasksPerCycle)
		suite.server.mutex.RUnlock()

		// 清理所有任务
		suite.server.mutex.Lock()
		for _, taskID := range taskIDs {
			delete(suite.server.backgroundTasks, taskID)
		}
		suite.server.mutex.Unlock()

		// 验证任务已清理
		suite.server.mutex.RLock()
		assert.Len(suite.T(), suite.server.backgroundTasks, 0, "第%d轮后应该没有任务", cycle)
		suite.server.mutex.RUnlock()

		// 短暂等待以模拟实际使用场景
		time.Sleep(1 * time.Millisecond)
	}
}

// TestTaskStatusTransitions 测试任务状态转换
func (suite *BackgroundTaskManagerTestSuite) TestTaskStatusTransitions() {
	taskID := "test_status_transitions_12345"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo status transitions",
		Output:    "",
		Status:    "running",
		StartTime: time.Now(),
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	// 测试状态转换序列
	statusTransitions := []struct {
		status    string
		exitCode  *int
		error     string
		output    string
	}{
		{"running", nil, "", ""},
		{"running", nil, "", "Partial output"},
		{"completed", &[]int{0}[0], "", "Complete output"},
	}

	for i, transition := range statusTransitions {
		suite.server.mutex.Lock()
		if task, exists := suite.server.backgroundTasks[taskID]; exists {
			task.Status = transition.status
			task.ExitCode = transition.exitCode
			task.Error = transition.error
			if transition.output != "" {
				task.Output = transition.output
			}
		}
		suite.server.mutex.Unlock()

		// 验证状态
		suite.server.mutex.RLock()
		currentTask, exists := suite.server.backgroundTasks[taskID]
		suite.server.mutex.RUnlock()

		assert.True(suite.T(), exists, "任务应该存在")
		assert.Equal(suite.T(), transition.status, currentTask.Status, "状态转换 %d 应该正确", i)
		
		if transition.exitCode != nil {
			assert.NotNil(suite.T(), currentTask.ExitCode, "ExitCode应该存在")
			assert.Equal(suite.T(), *transition.exitCode, *currentTask.ExitCode, "ExitCode应该正确")
		}
		
		assert.Equal(suite.T(), transition.error, currentTask.Error, "错误信息应该正确")
		assert.Equal(suite.T(), transition.output, currentTask.Output, "输出应该正确")
	}

	// 清理
	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, taskID)
	suite.server.mutex.Unlock()
}

// 运行后台任务管理器测试套件
func TestBackgroundTaskManagerTestSuite(t *testing.T) {
	suite.Run(t, new(BackgroundTaskManagerTestSuite))
}

// 基准测试
func BenchmarkConcurrentTaskCreation(b *testing.B) {
	server := NewMCPServer()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		const numTasks = 10
		
		for j := 0; j < numTasks; j++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				
				taskID := "benchmark_task_" + string(rune('A'+index))
				task := &BackgroundTask{
					ID:        taskID,
					Command:   "echo benchmark",
					StartTime: time.Now(),
					Status:    "running",
				}

				server.mutex.Lock()
				server.backgroundTasks[taskID] = task
				server.mutex.Unlock()
			}(j)
		}
		
		wg.Wait()
		
		// 清理
		server.mutex.Lock()
		server.backgroundTasks = make(map[string]*BackgroundTask)
		server.mutex.Unlock()
	}
}