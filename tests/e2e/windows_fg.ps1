Add-Type @"
using System; using System.Runtime.InteropServices;
public class F { [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
 [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid); }
"@
$h=[F]::GetForegroundWindow(); $p=0; [void][F]::GetWindowThreadProcessId($h,[ref]$p)
if ($p -gt 0) { (Get-Process -Id $p).ProcessName } else { "ninguna" }
