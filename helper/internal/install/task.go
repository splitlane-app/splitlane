package install

import (
	"encoding/xml"
	"strings"
	"unicode/utf16"
)

// TaskName is the scheduled task of the product (spec 4.3); the window starts it with schtasks /run.
const TaskName = "SplitLane Helper"

func esc(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// TaskXML is the task of spec 4.3, the XML of check (е) of Appendix A (plan 1A, Task 6) for the
// product: command without arguments, as the setup user (account, sid) in his session with the
// highest privileges; a logon trigger of that user; start on demand; no time limit, no power
// conditions; a second start while it runs is ignored (IgnoreNew). Priority 5 is the normal priority
// class: the helper runs at the class of the task and the core it starts at the helper's, and the core
// carries all traffic (7, the default of a task, is below normal). The user may read and run the task
// (FRFX), not change it.
func TaskXML(account, sid, command string) string {
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <URI>\` + esc(TaskName) + `</URI>
    <SecurityDescriptor>D:(A;;FA;;;BA)(A;;FA;;;SY)(A;;FRFX;;;` + esc(sid) + `)</SecurityDescriptor>
    <Description>SplitLane: the elevated helper, started at logon of the setup user and by the window</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>` + esc(account) + `</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>` + esc(account) + `</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>false</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings>
      <StopOnIdleEnd>false</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>5</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>` + esc(command) + `</Command>
    </Exec>
  </Actions>
</Task>
`
}

// utf16File is what schtasks /create /xml reads: UTF-16 LE with a byte order mark, CRLF lines.
func utf16File(text string) []byte {
	text = strings.ReplaceAll(text, "\n", "\r\n")
	units := utf16.Encode([]rune(text))
	b := []byte{0xFF, 0xFE}
	for _, u := range units {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}
