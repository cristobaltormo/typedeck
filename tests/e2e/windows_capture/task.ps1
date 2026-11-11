param([string]$Action, [string]$Name, [string]$Cmd)
if ($Action -eq "start") {
  schtasks /Create /TN $Name /SC ONCE /ST 23:59 /RU cristobal /IT /TR $Cmd /F | Out-Null
  schtasks /Run /TN $Name | Out-Null
} else {
  schtasks /End /TN $Name 2>$null | Out-Null
  schtasks /Delete /TN $Name /F 2>$null | Out-Null
}
