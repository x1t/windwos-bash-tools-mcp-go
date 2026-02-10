#!/usr/bin/env pwsh

# MCP Bash Tools 构建脚本
# 只构建64位Windows可执行文件到dist目录

param(
    [switch]$Clean,    # 清理构建缓存
    [switch]$Verbose,  # 详细输出
    [switch]$Release   # 发布模式构建
)

# 颜色输出函数
function Write-ColorOutput {
    param(
        [string]$Message,
        [string]$Color = "White"
    )
    Write-Host $Message -ForegroundColor $Color
}

# 错误处理
function Handle-Error {
    param([string]$ErrorMessage)
    Write-ColorOutput "❌ 错误: $ErrorMessage" "Red"
    exit 1
}

# 检查依赖
function Test-Dependencies {
    Write-ColorOutput "🔍 检查构建依赖..." "Yellow"
    
    # 检查Go版本
    try {
        $goVersion = go version 2>$null
        if ($LASTEXITCODE -ne 0) {
            Handle-Error "Go未安装或不在PATH中"
        }
        Write-ColorOutput "✅ Go版本: $goVersion" "Green"
    } catch {
        Handle-Error "无法执行go命令"
    }
    
    # 检查必要的Go模块
    if (-not (Test-Path "go.mod")) {
        Handle-Error "go.mod文件不存在"
    }
    Write-ColorOutput "✅ Go模块检查通过" "Green"
}

# 清理构建缓存
function Invoke-Clean {
    Write-ColorOutput "🧹 清理构建缓存..." "Yellow"
    
    # 清理dist目录
    if (Test-Path "dist") {
        Remove-Item -Recurse -Force "dist"
        Write-ColorOutput "✅ 已删除dist目录" "Green"
    }
    
    # 清理Go缓存
    if ($Clean) {
        go clean -cache 2>$null
        go clean -modcache 2>$null
        Write-ColorOutput "✅ 已清理Go缓存" "Green"
    }
}

# 创建目录结构
function New-DirectoryStructure {
    Write-ColorOutput "📁 创建目录结构..." "Yellow"
    
    # 创建dist目录
    if (-not (Test-Path "dist")) {
        New-Item -ItemType Directory -Path "dist" | Out-Null
        Write-ColorOutput "✅ 已创建dist目录" "Green"
    }
}

# 构建配置
function Get-BuildConfig {
    $buildMode = if ($Release) { "release" } else { "debug" }
    
    return @{
        OutputPath = "dist/bash-tools.exe"
        Goos = "windows"
        Goarch = "amd64"
        BuildMode = $buildMode
        UseLdflags = $Release
    }
}

# 执行构建
function Invoke-Build {
    param([hashtable]$Config)
    
    Write-ColorOutput "🔨 开始构建 ($($Config.BuildMode)模式)..." "Yellow"
    Write-ColorOutput "   目标: $($Config.Goos)/$($Config.Goarch)" "Cyan"
    Write-ColorOutput "   输出: $($Config.OutputPath)" "Cyan"
    
    # 设置环境变量
    $env:GOOS = $Config.Goos
    $env:GOARCH = $Config.Goarch
    $env:CGO_ENABLED = "0"
    
    # 构建命令
    $buildArgs = @(
        "build"
        "-o", $Config.OutputPath
        "./cmd/server"
    )
    
    if ($Config.Ldflags) {
        $buildArgs += @("-ldflags", $Config.Ldflags)
    }
    
    if ($Verbose) {
        $buildArgs += "-v"
    }
    
    # 执行构建
    $outputPath = $Config.OutputPath
    
    Write-ColorOutput "   构建目标: $outputPath" "Gray"
    Write-ColorOutput "   构建模式: $($Config.BuildMode)" "Gray"
    
    # 设置环境变量
    $env:GOOS = $Config.Goos
    $env:GOARCH = $Config.Goarch
    $env:CGO_ENABLED = "0"
    
    # 基础构建（暂时简化ldflags处理）
    if ($Config.UseLdflags) {
        Write-ColorOutput "   发布模式: 构建优化版本" "Yellow"
        # 简化发布模式，暂时不使用复杂的ldflags
        go build -ldflags "-s -w" -o $outputPath ./cmd/server
    } else {
        Write-ColorOutput "   调试模式: 基础构建" "Gray"
        go build -o $outputPath ./cmd/server
    }
    
    if ($LASTEXITCODE -ne 0) {
        Handle-Error "构建失败"
    }
    
    # 验证输出文件
    if (-not (Test-Path $Config.OutputPath)) {
        Handle-Error "构建输出文件不存在"
    }
    
    $fileInfo = Get-Item $Config.OutputPath
    Write-ColorOutput "✅ 构建成功!" "Green"
    Write-ColorOutput "   文件大小: $([math]::Round($fileInfo.Length / 1MB, 2)) MB" "Green"
    Write-ColorOutput "   创建时间: $($fileInfo.CreationTime)" "Green"
}

# 显示构建信息
function Show-BuildInfo {
    param([hashtable]$Config)
    
    Write-ColorOutput "" "White"
    Write-ColorOutput "📋 构建信息:" "Cyan"
    Write-ColorOutput "============" "Cyan"
    Write-ColorOutput "项目: MCP Bash Tools" "White"
    Write-ColorOutput "版本: $((Get-Date).ToString('yyyy.MM.dd-HHmm'))" "White"
    Write-ColorOutput "目标平台: $($Config.Goos)/$($Config.Goarch)" "White"
    Write-ColorOutput "构建模式: $($Config.BuildMode)" "White"
    Write-ColorOutput "输出路径: $($Config.OutputPath)" "White"
    Write-ColorOutput "" "White"
}

# 主函数
function Main {
    Write-ColorOutput "🚀 MCP Bash Tools 构建脚本" "Magenta"
    Write-ColorOutput "================================" "Magenta"
    
    try {
        # 检查依赖
        Test-Dependencies
        
        # 显示构建信息
        $config = Get-BuildConfig
        Show-BuildInfo -Config $config
        
        # 清理和准备
        Invoke-Clean
        New-DirectoryStructure
        
        # 执行构建
        Invoke-Build -Config $config
        
        Write-ColorOutput "" "White"
        Write-ColorOutput "🎉 构建完成!" "Green"
        Write-ColorOutput "可执行文件: $($config.OutputPath)" "Yellow"
        Write-ColorOutput "" "White"
        Write-ColorOutput "💡 使用方法:" "Cyan"
        Write-ColorOutput "   直接运行: .\$($config.OutputPath)" "Gray"
        Write-ColorOutput "   MCP配置:" "Gray"
        Write-ColorOutput "   `"mcpServers`": {" "Gray"
        Write-ColorOutput "     `"bash-tools`": {" "Gray"
        $jsonPath = (Resolve-Path $config.OutputPath).Path.Replace('\', '\\')
        Write-ColorOutput "       `"command`": `"$jsonPath`"" "Gray"
        Write-ColorOutput "     }" "Gray"
        Write-ColorOutput "   }" "Gray"
        
    } catch {
        Handle-Error "构建过程中发生错误: $($_.Exception.Message)"
    }
}

# 显示帮助
function Show-Help {
    Write-ColorOutput "用法: .\build.ps1 [参数]" "Cyan"
    Write-ColorOutput "" "White"
    Write-ColorOutput "参数:" "Yellow"
    Write-ColorOutput "  -Clean    清理构建缓存和dist目录" "White"
    Write-ColorOutput "  -Release  发布模式构建（优化和压缩）" "White"
    Write-ColorOutput "  -Verbose  详细输出构建过程" "White"
    Write-ColorOutput "  -Help     显示此帮助信息" "White"
    Write-ColorOutput "" "White"
    Write-ColorOutput "示例:" "Yellow"
    Write-ColorOutput "  .\build.ps1                    # 调试模式构建" "Gray"
    Write-ColorOutput "  .\build.ps1 -Release           # 发布模式构建" "Gray"
    Write-ColorOutput "  .\build.ps1 -Clean             # 清理并构建" "Gray"
    Write-ColorOutput "  .\build.ps1 -Release -Verbose  # 发布模式详细构建" "Gray"
}

# 参数处理
if ($args -contains "-Help" -or $args -contains "--help" -or $args -contains "-h") {
    Show-Help
    exit 0
}

# 执行主函数
Main
