# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 🚀 项目概述

这是一个**MCP Bash Tools**项目，基于Go语言实现，用于安全执行PowerShell/Bash命令。项目采用MCP（Model Context Protocol）协议，提供三个核心工具：

1. **Bash工具** - 安全执行PowerShell命令（支持超时、前台/后台执行）
2. **BashOutput工具** - 获取后台命令的实时输出（支持正则过滤）
3. **KillShell工具** - 终止后台运行的命令

## 📋 常用命令

### 构建项目
```powershell
# 调试模式构建（默认）
.\build.ps1

# 发布模式构建
.\build.ps1 -Release

# 清理缓存并重新构建
.\build.ps1 -Clean

# 详细输出模式
.\build.ps1 -Verbose

# 编译单个文件（测试用）
go build -o dist/bash-tools.exe ./cmd/server
```

### 运行和测试
```powershell
# 直接运行服务器
go run ./cmd/server

# 运行所有测试
go test ./...

# 运行特定包测试
go test ./internal/executor

# 运行测试并查看覆盖率
go test -cover ./...

# 运行单个测试文件
go test -v ./internal/security/security_test.go

# 性能测试
go test -bench=. ./...

# 启用竞态检测
go test -race ./...
```

### 代码质量检查
```powershell
# 格式化代码
go fmt ./...

# 静态分析
go vet ./...

# 下载依赖
go mod download

# 整理依赖
go mod tidy

# 检查模块依赖
go mod graph
```

## 🏗️ 架构设计

### 整体架构
```
┌─────────────────────────────────────────┐
│              cmd/server                 │
│           (MCP 服务器入口)               │
└────────────────┬────────────────────────┘
                 │
         ┌───────┴────────┬──────────────┐
         ▼                ▼              ▼
    ┌─────────┐    ┌──────────┐    ┌─────────┐
    │  Bash   │    │BashOutput│    │KillShell│
    │  Tool   │    │  Tool    │    │  Tool   │
    └────┬────┘    └─────┬────┘    └────┬────┘
         │               │               │
    ┌────┴────┐     ┌────┴──────┐  ┌────┴────┐
    │Executor │     │  后台任务  │  │ 任务管理 │
    │  组件    │     │  管理器    │  │  组件    │
    └────┬────┘     └───────────┘  └─────────┘
         │
    ┌────┴──────────────────┐
    │    核心模块层           │
    ├───────────────────────┤
    │ • internal/executor   │
    │ • internal/security   │
    │ • internal/windows    │
    └───────────────────────┘
```

### 核心模块详解

#### 1. **执行器层** (`internal/executor/`)
- `bash.go` - 基础Bash执行器
- `secure_bash.go` - **安全执行器**（核心模块）
  - 危险命令过滤（rm -rf, format, shutdown等）
  - 超时控制（1-600秒）
  - 前台/后台执行模式
- `shell.go` - Shell进程管理

#### 2. **安全模块** (`internal/security/`)
- `security.go` - 安全策略定义
- `validator.go` - 输入验证和命令检查
  - 白名单验证
  - 危险命令检测
  - 参数清理

#### 3. **Windows优化** (`internal/windows/`)
- `optimize.go` - Windows特定性能优化
- PowerShell 7集成

#### 4. **工具包** (`pkg/`)
- `config/` - 配置管理
- `errors/` - 错误处理
- `logger/` - 日志系统
- `utils/` - 通用工具

### MCP工具实现模式

所有MCP工具遵循统一模式：

```go
// 1. 定义工具
var BashTool = &spec.Tool{
    Name:        "bash",
    Description: "安全执行PowerShell命令",
    InputSchema: spec.InputSchema{
        Type:       "object",
        Properties: map[string]spec.JSONSchema{
            "command": {Type: "string", Description: "要执行的命令"},
            "timeout": {Type: "integer", Minimum: 1, Maximum: 600},
            "run_in_background": {Type: "boolean"},
        },
    },
}

// 2. 实现处理函数
func handleBash(ctx context.Context, args map[string]interface{}) (*spec.CallToolResult, error) {
    // 参数验证
    // 安全检查
    // 执行命令
    // 返回结果
}
```

## 🔧 开发指南

### 开发流程
1. **修改代码** → 2. **运行测试** → 3. **构建验证** → 4. **提交代码**

### 安全特性
- ✅ **命令白名单**: 仅允许预定义的安全命令
- ✅ **参数验证**: 所有输入参数严格验证
- ✅ **超时保护**: 防止命令无限执行
- ✅ **后台任务隔离**: 每个后台任务独立进程

### 任务管理
- 后台任务使用`map[string]*Task`存储
- 使用`sync.RWMutex`保证并发安全
- 任务状态：`pending` → `running` → `completed`/`failed`/`killed`

### 日志系统
- 使用结构化日志（`pkg/logger`）
- 级别：DEBUG, INFO, WARN, ERROR
- 所有操作都有日志记录

## 📁 重要文件位置

| 文件路径 | 描述 |
|---------|------|
| `cmd/server/main.go` | MCP服务器主程序 |
| `internal/executor/secure_bash.go` | 核心安全执行器 |
| `internal/security/security.go` | 安全策略 |
| `build.ps1` | 构建脚本 |
| `go.mod` | Go模块依赖 |
| `go-sdk/README.md` | MCP SDK文档 |

## 🎯 测试建议

### 测试覆盖重点
1. **安全模块测试** - 危险命令过滤
2. **执行器测试** - 超时、并发、前台/后台
3. **MCP工具测试** - 参数验证、错误处理
4. **集成测试** - 完整工作流

### 运行特定测试
```powershell
# 测试安全模块
go test -v ./internal/security

# 测试执行器
go test -race ./internal/executor

# 测试完整工具链
go test -v ./cmd/server
```

## 🚨 已知限制

- 仅支持**Windows x64**平台
- 需要**PowerShell 7**+
- 后台任务**最多50个**（可配置）
- 单次命令**最大超时600秒**

## 📚 参考文档

- `docs/README.md` - 完整文档
- `docs/protocol.md` - MCP协议说明
- `docs/server.md` - 服务器实现指南
- `design/design.md` - 架构设计文档
- `go-sdk/` - MCP Go SDK示例

## 🔍 故障排除

### 构建失败
```powershell
# 检查Go版本（需要1.21+）
go version

# 清理模块缓存
go clean -modcache
go mod download

# 重新构建
go build ./...
```

### 测试失败
```powershell
# 详细输出查看失败原因
go test -v -timeout 30s ./...

# 竞态条件检测
go test -race ./...
```

### 权限问题
- 确保PowerShell执行策略允许脚本运行
- 以管理员身份运行（如需系统级操作）

---

**💡 小贴士**: 这是一个成熟的企业级项目，代码质量高、文档完善。开发时请保持相同标准，确保所有新功能都有完整的测试覆盖和安全验证！🎉
