package rules

import "testing"

func TestProgramFolder(t *testing.T) {
	terminal := `C:\Program Files\WindowsApps\Microsoft.WindowsTerminal_1.21.3231.0_x64__8wekyb3d8bbwe\`
	cases := map[string]string{
		`C:\Games\Steam\steam.exe`:                               `C:\Games\Steam\`,
		`C:\Games\Steam\bin\cef\steamwebhelper.exe`:              `C:\Games\Steam\bin\cef\`,
		`C:\Users\u\AppData\Local\Discord\app-1.0.9\Discord.exe`: `C:\Users\u\AppData\Local\Discord\`,
		`C:\Users\u\AppData\Local\Discord\Update.exe`:            `C:\Users\u\AppData\Local\Discord\`,
		`C:\Apps\Browser\124.0.6367.91\browser.exe`:              `C:\Apps\Browser\`,
		terminal + `WindowsTerminal.exe`:                         terminal,
		// shared folders give the file itself
		`D:\tool.exe`:                         `D:\tool.exe`,
		`C:\Windows\explorer.exe`:             `C:\Windows\explorer.exe`,
		`C:\Windows\System32\curl.exe`:        `C:\Windows\System32\curl.exe`,
		`C:\Program Files\app.exe`:            `C:\Program Files\app.exe`,
		`C:\Program Files (x86)\app.exe`:      `C:\Program Files (x86)\app.exe`,
		`C:\Users\u\Downloads\setup.exe`:      `C:\Users\u\Downloads\setup.exe`,
		`C:\Users\u\AppData\Local\Temp\x.exe`: `C:\Users\u\AppData\Local\Temp\x.exe`,
		`C:\Users\u\Downloads\tool\tool.exe`:  `C:\Users\u\Downloads\tool\`,
		// paths that Normalize refuses are their own group
		`:System`:              `:System`,
		`Registry`:             `Registry`,
		`C:\Tools\script.cmd`:  `C:\Tools\script.cmd`,
		`\\server\share\a.exe`: `\\server\share\a.exe`,
	}
	for exe, want := range cases {
		if got := ProgramFolder(exe, testEnv); got != want {
			t.Errorf("%s: got %q, want %q", exe, got, want)
		}
	}
	calls := 0
	env := testEnv
	env.Describe = func(string) string { calls++; return "" }
	ProgramFolder(`C:\Games\Steam\steam.exe`, env)
	if calls != 0 {
		t.Error("ProgramFolder must not read the file's resources")
	}
}
