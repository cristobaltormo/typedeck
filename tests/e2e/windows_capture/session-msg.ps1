param([int]$Code)
Add-Type -Namespace T -Name W -MemberDefinition '[DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindow(string c, string t); [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);'
$h = [T.W]::FindWindow("TypedeckShell", [NullString]::Value)
"window $h"
if ($h -eq [IntPtr]::Zero) { exit 1 }
"posted " + [T.W]::PostMessage($h, 0x02B1, [IntPtr]$Code, [IntPtr]::Zero)
