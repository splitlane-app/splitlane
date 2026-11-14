package install

import (
	"encoding/xml"
	"strings"
	"testing"
	"unicode/utf16"
)

// The task of spec 4.3 is the XML of check (е) (plan 1A, Task 6), for the product: the task
// SplitLane Helper runs <ProgramDir>\splitlane-helper.exe without arguments, as the setup user with
// the highest privileges, at his logon and on demand, without a time limit or power conditions, a
// second start ignored; he may read and run it, not change it. The file is UTF-16 LE with a BOM.
func TestTaskXMLProduct(t *testing.T) {
	const sid = "S-1-5-21-1000-2000-3000-1001"
	text := TaskXML(`TESTPC\Тестер & Co`, sid, `C:\Program Files\SplitLane\splitlane-helper.exe`)
	var task struct {
		Registration struct {
			URI string `xml:"URI"`
			SD  string `xml:"SecurityDescriptor"`
		} `xml:"RegistrationInfo"`
		Trigger struct {
			Enabled bool   `xml:"Enabled"`
			User    string `xml:"UserId"`
		} `xml:"Triggers>LogonTrigger"`
		Principal struct {
			User     string `xml:"UserId"`
			Logon    string `xml:"LogonType"`
			RunLevel string `xml:"RunLevel"`
		} `xml:"Principals>Principal"`
		Settings struct {
			Multiple    string `xml:"MultipleInstancesPolicy"`
			NoBattery   bool   `xml:"DisallowStartIfOnBatteries"`
			StopBattery bool   `xml:"StopIfGoingOnBatteries"`
			OnDemand    bool   `xml:"AllowStartOnDemand"`
			Enabled     bool   `xml:"Enabled"`
			TimeLimit   string `xml:"ExecutionTimeLimit"`
		} `xml:"Settings"`
		Exec struct {
			Command   string `xml:"Command"`
			Arguments string `xml:"Arguments"`
		} `xml:"Actions>Exec"`
	}
	// encoding/xml refuses a declared UTF-16 encoding for a Go string; parse the body only.
	body := text[strings.Index(text, "<Task"):]
	if err := xml.Unmarshal([]byte(body), &task); err != nil {
		t.Fatal(err)
	}
	switch {
	case TaskName != "SplitLane Helper" || task.Registration.URI != `\SplitLane Helper`:
		t.Fatal(task.Registration.URI)
	case task.Registration.SD != "D:(A;;FA;;;BA)(A;;FA;;;SY)(A;;FRFX;;;"+sid+")":
		t.Fatal(task.Registration.SD)
	case !task.Trigger.Enabled || task.Trigger.User != `TESTPC\Тестер & Co` || task.Principal.User != task.Trigger.User:
		t.Fatalf("%+v %+v", task.Trigger, task.Principal)
	case task.Principal.Logon != "InteractiveToken" || task.Principal.RunLevel != "HighestAvailable":
		t.Fatalf("%+v", task.Principal)
	case task.Settings.Multiple != "IgnoreNew" || task.Settings.NoBattery || task.Settings.StopBattery ||
		!task.Settings.OnDemand || !task.Settings.Enabled || task.Settings.TimeLimit != "PT0S":
		t.Fatalf("%+v", task.Settings)
	case task.Exec.Command != `C:\Program Files\SplitLane\splitlane-helper.exe` || task.Exec.Arguments != "" || strings.Contains(text, "<Arguments>"):
		t.Fatalf("%+v", task.Exec)
	}

	b := utf16File("a\nя")
	if b[0] != 0xFF || b[1] != 0xFE {
		t.Fatal("no BOM")
	}
	units := make([]uint16, (len(b)-2)/2)
	for i := range units {
		units[i] = uint16(b[2+2*i]) | uint16(b[3+2*i])<<8
	}
	if string(utf16.Decode(units)) != "a\r\nя" {
		t.Fatalf("%q", string(utf16.Decode(units)))
	}
}
