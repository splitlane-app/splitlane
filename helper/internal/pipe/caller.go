package pipe

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/place"
)

// Caller is the program on the other end of a pipe connection.
type Caller struct {
	PID      uint32
	Image    string // the full path of the image
	Elevated bool   // its token is elevated
}

// Check decides whether a connection is served; nil admits the client.
type Check func(conn net.Conn) error

// Allowed admits an elevated client, or one whose image lies inside productDir (case does not
// folds for ASCII letters only; productDir is a folder boundary: SplitLaneX is not inside SplitLane).
func Allowed(c Caller, productDir string) bool {
	if c.Elevated {
		return true
	}
	return filepath.IsAbs(c.Image) && place.PathInside(c.Image, productDir)
}

// ProductCheck is the check of the product: the client is elevated or part of the product folder.
// The error names the base name of the image and the PID, never a full path.
func ProductCheck(productDir string) Check {
	return func(conn net.Conn) error {
		c, err := ClientOf(conn)
		if err != nil {
			return fmt.Errorf("the client process: %w", err)
		}
		if !Allowed(c, productDir) {
			return fmt.Errorf("not part of SplitLane: %s (process %d)", filepath.Base(c.Image), c.PID)
		}
		return nil
	}
}

type fder interface{ Fd() uintptr }

// ClientOf finds the process at the client end of a go-winio pipe connection. Known limitation: the image
// comes as the system returns it; an 8.3 short name never matches the product folder (it fails closed).
func ClientOf(conn net.Conn) (Caller, error) {
	f, ok := conn.(fder)
	if !ok {
		return Caller{}, errors.New("the connection has no handle")
	}
	var pid uint32
	if err := windows.GetNamedPipeClientProcessId(windows.Handle(f.Fd()), &pid); err != nil {
		return Caller{}, fmt.Errorf("client process id: %w", err)
	}
	c := Caller{PID: pid}
	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return c, fmt.Errorf("open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(proc)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(proc, 0, &buf[0], &n); err != nil {
		return c, fmt.Errorf("image of process %d: %w", pid, err)
	}
	c.Image = windows.UTF16ToString(buf[:n])
	var tok windows.Token
	if err := windows.OpenProcessToken(proc, windows.TOKEN_QUERY, &tok); err != nil {
		return c, fmt.Errorf("token of process %d: %w", pid, err)
	}
	defer tok.Close()
	c.Elevated = tok.IsElevated()
	return c, nil
}
