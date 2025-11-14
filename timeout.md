根据搜索结果，PowerShell有几种实现超时的方法：

  📚 PowerShell超时方法总结

  1️⃣ Start-Job + Wait-Job -Timeout（推荐）

  Start-Job { Your-Command-Here } | Wait-Job -Timeout 3

  2️⃣ Wait-Process -Timeout

  $process = Start-Process -FilePath "notepad.exe" -PassThru
  Wait-Process -Name "notepad" -Timeout 60

  3️⃣ Invoke-Command -Timeout

  try {
      Invoke-Command -ScriptBlock { Start-Sleep -Seconds 100 } -Timeout 30
  } catch {
      Write-Host "The command timed out."
  }
