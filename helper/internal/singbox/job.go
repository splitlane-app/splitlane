package singbox

import (
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// helperJob is the job object of the helper's `sing-box api ...` children (review F3-2). Its limit
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE ends them when its last handle closes; the helper never
// closes its handle, so the system closes it when the helper ends, also by a crash. A ping left
// behind (up to 50 s) would be counted by winhost.CoreRunning as a running core, and the re-enable
// of the next helper (5.7) would fail with other_core. The core itself is never put there: it
// outlives the helper on purpose (5.7).
var helperJob struct {
	once sync.Once
	h    windows.Handle
	err  error
}

// newKillJob creates a job object whose processes end when its last handle closes.
func newKillJob() (windows.Handle, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(h)
		return 0, err
	}
	return h, nil
}

// assignTo puts a started child into job. The child is not waited for yet, so its pid still names it.
func assignTo(job windows.Handle, p *os.Process) error {
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(p.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.AssignProcessToJobObject(job, h)
}

// inHelperJob puts a started child into the helper's job, created at the first call.
func inHelperJob(p *os.Process) error {
	helperJob.once.Do(func() { helperJob.h, helperJob.err = newKillJob() })
	if helperJob.err != nil {
		return helperJob.err
	}
	return assignTo(helperJob.h, p)
}
