$csc = "C:\Windows\Microsoft.NET\Framework64\v4.0.30319\csc.exe"
$dir = Split-Path -Parent $MyInvocation.MyCommand.Path
& $csc /nologo /target:winexe /out:"$dir\sink.exe" /r:System.Windows.Forms.dll /r:System.Drawing.dll "$dir\sink.cs"
