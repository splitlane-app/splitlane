package place

import "testing"

func TestPathInside(t *testing.T) {
	const dir = `C:\Program Files\SplitLane`
	cases := []struct {
		path string
		want bool
	}{
		{`C:\Program Files\SplitLane\a.exe`, true},
		{`c:\PROGRAM FILES\splitlane\helper\A.EXE`, true},
		{`C:\Program Files\SplitLane\helper\..\a.exe`, true},
		{`C:/Program Files/SplitLane/a.exe`, true},
		{dir, false},
		{dir + `\`, false},
		{`C:\Program Files\SplitLaneX\a.exe`, false},
		{"C:\\Program F\u0130les\\SplitLane\\a.exe", false}, // U+0130 is not "i" on NTFS
		{"C:\\Program Files\\Spl\u0130tLane\\a.exe", false},
		{"C:\\Program File\u017f\\SplitLane\\a.exe", false}, // U+017F folds to "s" in Go
		{"C:\\Program Files\\Split\u212aane\\a.exe", false}, // U+212A folds to "k" in Go
		{`D:\Program Files\SplitLane\a.exe`, false},
		{`SplitLane\a.exe`, false},
		{``, false},
	}
	for _, c := range cases {
		if got := PathInside(c.path, dir); got != c.want {
			t.Errorf("PathInside(%q) = %v, want %v", c.path, got, c.want)
		}
		if got := PathInside(c.path, dir+`\`); got != c.want {
			t.Errorf("PathInside(%q, trailing separator) = %v, want %v", c.path, got, c.want)
		}
	}
	if PathInside(`C:\a.exe`, "") || !PathInside(`C:\a.exe`, `C:\`) {
		t.Error("an empty folder contains nothing; the drive root contains its files")
	}
}

func TestPathEqual(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{`C:\X\Y`, `c:\x\y\`, true},
		{`C:\X\Y`, `C:\X\Z`, false},
		{"C:\\X\\\u0130", `C:\X\i`, false},
		{"C:\\X\\\u017f", `C:\X\s`, false},
		{"C:\\X\\\u212a", `C:\X\k`, false},
		{"C:\\X\\\u0130", "C:\\X\\\u0130", true},
		{``, ``, false},
	}
	for _, c := range cases {
		if got := PathEqual(c.a, c.b); got != c.want {
			t.Errorf("PathEqual(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
