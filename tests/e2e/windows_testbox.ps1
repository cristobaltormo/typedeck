param([string]$Dir = "C:\typedeck")
Add-Type -AssemblyName System.Windows.Forms
$form = New-Object System.Windows.Forms.Form
$form.Text = "Typedeck test box"
$form.Width = 520; $form.Height = 260; $form.TopMost = $true
$box = New-Object System.Windows.Forms.TextBox
$box.Multiline = $true; $box.Dock = "Fill"; $box.Font = New-Object System.Drawing.Font("Consolas", 12)
$form.Controls.Add($box)
$keys = Join-Path $Dir "testbox.keys"
Set-Content -Path $keys -Value "" -Encoding UTF8
$box.Add_KeyDown({
  param($s, $e)
  Add-Content -Path $keys -Value ("{0}|ctrl={1}|alt={2}|shift={3}" -f $e.KeyCode, $e.Control, $e.Alt, $e.Shift) -Encoding UTF8
  if ($e.Control -and $e.KeyCode -eq "A") { $box.SelectAll(); $e.SuppressKeyPress = $true }
})
$timer = New-Object System.Windows.Forms.Timer
$timer.Interval = 150
$timer.Add_Tick({ [IO.File]::WriteAllText((Join-Path $Dir "testbox.txt"), $box.Text, [Text.Encoding]::UTF8) })
$timer.Start()
$form.Add_Shown({ $form.Activate(); $box.Focus() })
[void]$form.ShowDialog()
