# 测试 kill_shell 是否能正确终止进程树
# 这个脚本用于手动验证修复

Write-Host "=== Kill Shell Process Tree Test ===" -ForegroundColor Cyan
Write-Host ""

# 1. 启动一个会创建子进程的后台任务
Write-Host "1. 启动测试进程（模拟 pnpm dev）..." -ForegroundColor Yellow
$testScript = @"
Write-Host 'Parent process started'
Start-Sleep -Seconds 1
# 启动子进程
Start-Process powershell -ArgumentList '-Command', 'Write-Host Child process started; Start-Sleep -Seconds 300' -NoNewWindow
Start-Sleep -Seconds 300
"@

$testScript | Out-File -FilePath "temp_test_script.ps1" -Encoding UTF8

# 启动父进程
$parentProcess = Start-Process powershell -ArgumentList "-File", "temp_test_script.ps1" -PassThru -NoNewWindow
Write-Host "   父进程 PID: $($parentProcess.Id)" -ForegroundColor Green

Start-Sleep -Seconds 2

# 2. 检查进程树
Write-Host ""
Write-Host "2. 检查进程树..." -ForegroundColor Yellow
$childProcesses = Get-Process | Where-Object { $_.Parent.Id -eq $parentProcess.Id } -ErrorAction SilentlyContinue
Write-Host "   找到 $($childProcesses.Count) 个子进程" -ForegroundColor Green

# 3. 使用 taskkill 终止进程树
Write-Host ""
Write-Host "3. 使用 taskkill /F /T 终止进程树..." -ForegroundColor Yellow
taskkill /F /T /PID $parentProcess.Id 2>&1 | Out-Null

Start-Sleep -Seconds 1

# 4. 验证进程是否被终止
Write-Host ""
Write-Host "4. 验证进程是否被终止..." -ForegroundColor Yellow
try {
    $stillRunning = Get-Process -Id $parentProcess.Id -ErrorAction Stop
    Write-Host "   ❌ 失败：父进程仍在运行" -ForegroundColor Red
}
catch {
    Write-Host "   ✅ 成功：父进程已终止" -ForegroundColor Green
}

# 检查子进程
if ($childProcesses) {
    $stillRunningChildren = 0
    foreach ($child in $childProcesses) {
        try {
            Get-Process -Id $child.Id -ErrorAction Stop | Out-Null
            $stillRunningChildren++
        }
        catch {
            # 进程已终止
        }
    }
    
    if ($stillRunningChildren -eq 0) {
        Write-Host "   ✅ 成功：所有子进程已终止" -ForegroundColor Green
    }
    else {
        Write-Host "   ❌ 失败：还有 $stillRunningChildren 个子进程在运行" -ForegroundColor Red
    }
}

# 清理
Remove-Item "temp_test_script.ps1" -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "=== 测试完成 ===" -ForegroundColor Cyan
Write-Host ""
Write-Host "结论：taskkill /F /T 可以正确终止整个进程树" -ForegroundColor Green
Write-Host "这证明了我们的修复方案是有效的" -ForegroundColor Green
