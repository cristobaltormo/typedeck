package platform

import (
	"bytes"
	"encoding/xml"
	"unicode/utf16"
)

const taskTemplate = `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Starts Typedeck when you sign in.</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>{USER}</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>{USER}</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>3</Count>
    </RestartOnFailure>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>{EXE}</Command>
      <Arguments>--background</Arguments>
    </Exec>
  </Actions>
</Task>
`

func xmlText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func taskXML(user, exe string) string {
	r := bytes.ReplaceAll([]byte(taskTemplate), []byte("{USER}"), []byte(xmlText(user)))
	return string(bytes.ReplaceAll(r, []byte("{EXE}"), []byte(xmlText(exe))))
}

func utf16WithBOM(s string) []byte {
	u := append([]uint16{0xFEFF}, utf16.Encode([]rune(s))...)
	out := make([]byte, 0, len(u)*2)
	for _, c := range u {
		out = append(out, byte(c), byte(c>>8))
	}
	return out
}
