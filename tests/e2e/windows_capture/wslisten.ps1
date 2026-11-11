param([int]$Seconds, [string]$Out)
$port = (Get-Content C:\td-capture\appdata\typedeck\port).Trim()
$base = "http://127.0.0.1:$port"
$page = Invoke-WebRequest -UseBasicParsing "$base/"
$token = [regex]::Match($page.Content, 'name="token" content="([^"]+)"').Groups[1].Value
$ws = New-Object System.Net.WebSockets.ClientWebSocket
$ws.Options.AddSubProtocol("typedeck.$token")
$ws.Options.SetRequestHeader("Origin", $base)
$ws.ConnectAsync([Uri]"ws://127.0.0.1:$port/api/ws", [Threading.CancellationToken]::None).Wait()
$buf = New-Object byte[] 8192
$end = (Get-Date).AddSeconds($Seconds)
"" | Set-Content $Out
"open" | Add-Content $Out
$pending = $null
while ((Get-Date) -lt $end) {
    if (-not $pending) { $pending = $ws.ReceiveAsync([ArraySegment[byte]]$buf, [Threading.CancellationToken]::None) }
    if ($pending.Wait(300)) {
        [Text.Encoding]::UTF8.GetString($buf, 0, $pending.Result.Count) | Add-Content $Out
        $pending = $null
    }
}
$ws.Dispose()
"closed" | Add-Content $Out
