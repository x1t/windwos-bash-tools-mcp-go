# Kill Shell Bug - 快速修复参考

## 问题
`kill_shell` 无法终止后台任务的子进程（如 `pnpm dev` 启动的 Vite 服务器）

## 原因
Windows 上 `Process.Kill()` 只终止父进程，不终止子进程

## 解决方案
使用 `taskkill /F /T /PID` 终止整个进程树

## 修复的代码位置

### 1. KillShellHandler (cmd/server/main.go:440-455)
```go
// 修复前
if process != nil {
    process.Kill()  // ❌ 只终止父进程
}

// 修复后
if process != nil {
    killCmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", process.Pid))
    if err := killCmd.Run(); err != nil {
        process.Kill()  // 回退机制
    }
}
```

### 2. handleCommandCancellation (cmd/server/main.go:680-690)
```go
// 修复前
if cmd.Process != nil {
    cmd.Process.Kill()  // ❌ 只终止父进程
}

// 修复后
if cmd.Process != nil {
    killCmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", cmd.Process.Pid))
    if err := killCmd.Run(); err != nil {
        cmd.Process.Kill()  // 回退机制
    }
}
```

## 验证步骤

### 1. 编译
```bash
go build -o mcp-bash-tools.exe ./cmd/server
```

### 2. 运行测试
```bash
go test -v ./cmd/server -run TestKillShellHandler
```

### 3. 手动测试
```bash
# 启动后台任务
# 使用 bash 工具: run_in_background=true, command="pnpm dev"

# 检查进程
tasklist | findstr "node"

# 终止任务
# 使用 kill_shell 工具: shell_id="<task_id>"

# 验证进程已终止
tasklist | findstr "node"  # 应该没有输出
```

## 关键点

✅ 使用 `taskkill /F /T` 终止进程树  
✅ 有回退机制（taskkill 失败时使用 process.Kill()）  
✅ 详细的错误日志  
✅ 修复了 context leak  
✅ 所有测试通过  

## 影响

- **修复前**: 子进程继续运行，资源泄漏
- **修复后**: 整个进程树被终止，资源正确释放

## 性能

- **额外开销**: ~10-50ms（调用 taskkill）
- **权衡**: 完全可接受，避免了进程泄漏

## 相关文档

- 详细审计: [KILL_SHELL_BUG_AUDIT.md](KILL_SHELL_BUG_AUDIT.md)
- 审计总结: [AUDIT_SUMMARY_2026-02-01.md](AUDIT_SUMMARY_2026-02-01.md)
- 改进记录: [../IMPROVEMENTS.md](../IMPROVEMENTS.md)
