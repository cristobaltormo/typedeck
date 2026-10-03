param([string]$Port = "COM4")
$p = New-Object System.IO.Ports.SerialPort $Port, 115200
$p.DtrEnable = $true
$p.ReadTimeout = 1500
$p.Open()
Start-Sleep -Milliseconds 400
$null = $p.ReadExisting()
$p.WriteLine("STATS")
Start-Sleep -Milliseconds 800
$p.ReadExisting()
$p.Close()
