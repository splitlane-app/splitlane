package atomicfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteCreatesAndReplacesWithoutBOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lane-vpn.json")
	if err := Write(path, []byte(`{"a":"Кириллица"}`)); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte(`{"b":2}`)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"b":2}` {
		t.Fatalf("content %q", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}

func TestWriteRefusesBOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	err := Write(path, append([]byte{0xEF, 0xBB, 0xBF}, '{', '}'))
	if err == nil || !strings.Contains(err.Error(), "byte order mark") {
		t.Fatalf("want a BOM error, got %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("target must not exist, stat: %v", err)
	}
}

func TestWriteRetriesWhileReaderHoldsTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lane-direct.json")
	if err := Write(path, []byte("old")); err != nil {
		t.Fatal(err)
	}
	// os.Open shares read and write but not delete, like a reader that blocks the rename.
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		f.Close()
	}()
	if err := Write(path, []byte("new")); err != nil {
		t.Fatalf("write while held: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Fatalf("content %q", got)
	}
}

func TestWriteGivesUpAfterAttempts(t *testing.T) {
	oldAttempts, oldPause := Attempts, Pause
	Attempts, Pause = 3, 10*time.Millisecond
	defer func() { Attempts, Pause = oldAttempts, oldPause }()
	dir := t.TempDir()
	path := filepath.Join(dir, "held.json")
	if err := Write(path, []byte("old")); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := Write(path, []byte("new")); err == nil {
		t.Fatal("want an error while the target stays held")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}
