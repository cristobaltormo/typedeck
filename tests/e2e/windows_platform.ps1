param(
  [Parameter(Mandatory = $true)][string]$TestExe,
  [Parameter(Mandatory = $true)][string]$Dir,
  [string]$Run = "TestE2E"
)
New-Item -ItemType Directory -Force $Dir | Out-Null
$box = Join-Path $PSScriptRoot "windows_testbox.ps1"
if (-not (Test-Path $box)) { $box = Join-Path $Dir "windows_testbox.ps1" }
$proc = Start-Process powershell -ArgumentList "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $box, "-Dir", $Dir -PassThru
$shell = New-Object -ComObject WScript.Shell
$ready = $false
for ($i = 0; $i -lt 25 -and -not $ready; $i++) {
  Start-Sleep -Milliseconds 600
  $shell.SendKeys("%")
  [void]$shell.AppActivate("Typedeck test box")
  $ready = [bool](Get-Process | Where-Object { $_.MainWindowTitle -eq "Typedeck test box" })
}
if (-not $ready) { Write-Output "FAIL the test box did not open"; exit 1 }
Start-Sleep -Seconds 1
$env:TYPEDECK_E2E_DIR = $Dir
$env:APPDATA = Join-Path $Dir "roaming"
$env:LOCALAPPDATA = Join-Path $Dir "local"
& $TestExe "-test.v" "-test.run" $Run 2>&1 | ForEach-Object { $_ }
$code = $LASTEXITCODE
Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
Get-Process | Where-Object { $_.MainWindowTitle -eq "Typedeck test box" } | Stop-Process -Force -ErrorAction SilentlyContinue
exit $code
