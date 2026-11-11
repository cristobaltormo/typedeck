param([string]$Path, [string]$Method = "GET", [string]$BodyFile = "")
$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$base = "http://127.0.0.1:7788"
$page = Invoke-WebRequest -UseBasicParsing "$base/"
$token = [regex]::Match($page.Content, 'name="token" content="([^"]+)"').Groups[1].Value
$call = @{ UseBasicParsing = $true; Method = $Method; Headers = @{ "X-Token" = $token }; Uri = "$base$Path" }
if ($BodyFile) {
    $call.ContentType = "application/json"
    $call.Body = [IO.File]::ReadAllText($BodyFile)
}
try {
    (Invoke-WebRequest @call).Content
} catch {
    $resp = $_.Exception.Response
    if ($resp) { (New-Object IO.StreamReader($resp.GetResponseStream())).ReadToEnd() } else { $_.Exception.Message }
}
