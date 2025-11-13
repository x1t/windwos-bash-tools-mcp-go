package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"mcp-bash-tools/internal/executor"
)

func main() {
	// 简单测试后台任务功能是否正常工作
	fmt.Println("Testing background task output fix...")
	
	// 创建一个简单的后台任务示例
	command := "echo 'Hello from background task'; sleep 2; echo 'Task completed'"
	
	// 使用BashExecutor进行测试
	executor := executor.NewBashExecutor()
	
	// 测试流式执行功能
	var output string
	var exitCode int
	var err error
	
	done := make(chan bool, 1)
	go func() {
		output, exitCode, err = executor.ExecuteWithStreaming(command, 5000, 
			func(line string) {
				fmt.Printf("Streaming output: %s\n", line)
			})
		done <- true
	}()
	
	select {
	case <-done:
		fmt.Printf("Command completed:\n")
		fmt.Printf("Output: %s\n", output)
		fmt.Printf("Exit Code: %d\n", exitCode)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	case <-time.After(6 * time.Second): // 稍微长于超时时间
		fmt.Println("Command timed out")
	}
	
	fmt.Println("\nTest completed. The fix should now properly handle background task output using temporary files.")
}