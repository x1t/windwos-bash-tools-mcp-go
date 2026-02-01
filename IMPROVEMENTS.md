# MCP Bash Tools - 改进记录

## 2026-02-01: 修复 kill_shell 无法终止进程树的 Bug

### 问题描述
用户报告使用 `kill_shell` 工具终止后台任务（如 `pnpm dev`）后，子进程（Vite 开发服务器）仍在运行，端口仍可访问。

### 根本原因
在 Windows 系统上，Go 的 `Process.Kill()` 只终止直接调用的进程，不会自动终止子进程。当执行 `pnpm dev` 时：
```
PowerShell.exe (父进程) ← 只有这个被终止
  └─ pnpm.exe          ← 继续运行
      └─ node.exe       ← 继续运行
          └─ vite       ← 继续运行（端口仍可访问）
```

### 解决方案
使用 Windows 原生的 `taskkill /F /T /PID` 命令来终止整个进程树：
- `/F` - 强制终止
- `/T` - 终止进程树（包括所有子进程）
- `/PID` - 指定进程 ID

### 修改的文件
1. `cmd/server/main.go`
   - `KillShellHandler`: 使用 taskkill 终止进程树
   - `handleCommandCancellation`: 使用 taskkill 终止进程树
   
2. `cmd/server/kill_shell_test.go`
   - 更新测试用例，创建实际的进程进行测试
   - 添加进程终止验证

### 技术细节
- **回退机制**: 如果 taskkill 失败（进程已退出、权限不足等），自动回退到 `process.Kill()`
- **错误处理**: 优雅处理各种边缘情况
- **日志记录**: 详细记录终止过程，便于调试

### 测试验证
```bash
# 运行测试
go test -v ./cmd/server -run TestKillShellHandler

# 所有测试通过 ✅
```

### 相关文档
详细的审计报告请参阅：[docs/KILL_SHELL_BUG_AUDIT.md](docs/KILL_SHELL_BUG_AUDIT.md)

---

## 未来改进建议

1. **跨平台支持**: 为 Linux/macOS 实现类似的进程树终止（使用进程组）
2. **进程监控**: 定期检查后台任务的进程是否仍在运行
3. **进程泄漏检测**: 启动时检查是否有遗留的孤儿进程
4. **超时保护**: 如果 taskkill 超时，记录警告
5. **用户反馈**: 在 kill_shell 返回时明确告知用户进程树已终止
