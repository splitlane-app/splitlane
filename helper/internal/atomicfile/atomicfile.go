// Package atomicfile writes the files that the core and the helper read back:
// UTF-8 without a byte order mark, a temp file in the same folder, flushed to
// disk, then renamed over the target (spec 6.5, 5.7).
package atomicfile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

// Attempts and Pause bound the retries of the final rename: the core may hold
// the target open for reading at that moment (spec 6.5).
var (
	Attempts = 40
	Pause    = 50 * time.Millisecond
)

var bom = []byte{0xEF, 0xBB, 0xBF}

// Write replaces path with data. data must not start with a UTF-8 byte order mark.
func Write(path string, data []byte) error {
	if bytes.HasPrefix(data, bom) {
		return errors.New("atomicfile: data starts with a byte order mark")
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // nothing left behind on failure; a no-op after the rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	from, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	for attempt := 1; ; attempt++ {
		err = windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if err == nil {
			return nil
		}
		if attempt >= Attempts || !retryable(err) {
			return fmt.Errorf("atomicfile: replace %s: %w", path, err)
		}
		time.Sleep(Pause)
	}
}

func retryable(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
