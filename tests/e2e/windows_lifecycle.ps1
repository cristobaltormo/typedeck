param(
  [Parameter(Mandatory = $true)][string]$Exe,
  [Parameter(Mandatory = $true)][string]$CliExe,
  [Parameter(Mandatory = $true)][string]$Dir
)
$fails = 0
function Check($name, $ok, $extra = "") {
  if ($ok) { "PASS  $name" } else { $script:fails++; "FAIL  $name   [$extra]" }
}
$env:APPDATA = Join-Path $Dir "roaming"
$env:LOCALAPPDATA = Join-Path $Dir "local"
Remove-Item -Recurse -Force $Dir -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $env:APPDATA, $env:LOCALAPPDATA | Out-Null
$log = Join-Path $env:LOCALAPPDATA "typedeck\typedeck.log"
$name = [IO.Path]::GetFileNameWithoutExtension($Exe)
function Running { @(Get-Process -Name $name -ErrorAction SilentlyContinue).Count }

Start-Process $Exe -ArgumentList "--background"
Start-Sleep 3
Check "the program starts and writes its log under the local profile" ((Running) -eq 1 -and (Test-Path $log))

$out = & $CliExe 2>&1 | Out-String
Check "a second copy exits at once and says where the editor is" ((Running) -eq 1 -and $LASTEXITCODE -eq 0 -and $out -match "already running")

taskkill /IM "$name.exe" | Out-Null
Start-Sleep 3
$text = Get-Content $log -Raw
Check "taskkill without /F stops it" ((Running) -eq 0)
Check "the orderly shutdown ran (board released, statistics saved)" ($text -match "shutting down" -and $text -match "stopped")

Start-Process $Exe -ArgumentList "--background"
Start-Sleep 3
$sw = [Diagnostics.Stopwatch]::StartNew()
$out = & $CliExe uninstall 2>&1 | Out-String
Check "uninstall stops a running copy in an orderly way and quickly" ((Running) -eq 0 -and $sw.Elapsed.TotalSeconds -lt 6) "$($sw.Elapsed.TotalSeconds)s"

$first = & $CliExe install 2>&1 | Out-String
$code1 = $LASTEXITCODE
$second = & $CliExe install 2>&1 | Out-String
Check "install twice is fine" ($code1 -eq 0 -and $LASTEXITCODE -eq 0) $second
$xml = schtasks /Query /TN Typedeck /XML | Out-String
Check "the task has no execution time limit" ($xml -match "<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>")
Check "the task runs on battery" ($xml -match "<DisallowStartIfOnBatteries>false" -and $xml -match "<StopIfGoingOnBatteries>false")
Check "the task never starts a second copy" ($xml -match "<MultipleInstancesPolicy>IgnoreNew")
Check "the task restarts after a failure" ($xml -match "<RestartOnFailure>")
Check "the task starts with the session" ($xml -match "<LogonTrigger>" -and $xml -match "--background")
Start-Sleep 2
Get-Process -Name $name -ErrorAction SilentlyContinue | Stop-Process -Force
& $CliExe uninstall | Out-Null
$code1 = $LASTEXITCODE
& $CliExe uninstall | Out-Null
Check "uninstall twice is fine" ($code1 -eq 0 -and $LASTEXITCODE -eq 0)
schtasks /Query /TN Typedeck 2>&1 | Out-Null
Check "the task is gone after uninstall (checked by exit code, not by text)" ($LASTEXITCODE -ne 0)

$doctor = & $CliExe doctor 2>&1 | Out-String
Check "doctor from a console writes its report and returns an exit code" ($doctor -match "Typedeck" -and $doctor -match "taskkill" -and ($LASTEXITCODE -eq 0 -or $LASTEXITCODE -eq 1))
& $CliExe nonsense 2>&1 | Out-Null
Check "an unknown command returns 2" ($LASTEXITCODE -eq 2)

Get-Process -Name "typedeck-cli","typedeck" -ErrorAction SilentlyContinue | Stop-Process -Force

if ($fails -gt 0) { "$fails lifecycle checks failed"; exit 1 }
"all lifecycle checks passed"
