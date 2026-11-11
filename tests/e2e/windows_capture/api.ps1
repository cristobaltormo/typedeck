param([string]$Path, [string]$Method = "GET", [string]$BodyFile = "")
$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$port = (Get-Content C:\td-capture\appdata\typedeck\port).Trim()
$base = "http://127.0.0.1:$port"
$page = Invoke-WebRequest -UseBasicParsing "$base/"
$token = [regex]::Match($page.Content, 'name="token" content="([^"]+)"').Groups[1].Value
$args2 = @{ UseBasicParsing = $true; Method = $Method; Headers = @{ "X-Token" = $token }; Uri = "$base$Path" }
if ($BodyFile) {
    $args2.ContentType = "application/json"
    $args2.Body = [IO.File]::ReadAllText($BodyFile)
}
try {
    $r = Invoke-WebRequest @args2
    Write-Output ("HTTP " + [int]$r.StatusCode)
    Write-Output $r.Content
} catch {
    $resp = $_.Exception.Response
    if ($resp) {
        $reader = New-Object IO.StreamReader($resp.GetResponseStream())
        Write-Output ("HTTP " + [int]$resp.StatusCode)
        Write-Output $reader.ReadToEnd()
    } else {
        Write-Output ("HTTP 0")
        Write-Output $_.Exception.Message
    }
}
