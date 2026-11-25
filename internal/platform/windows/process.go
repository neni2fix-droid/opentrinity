//go:build windows

// Package windows contains Windows-specific process containment helpers.
package windows

import (
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CREATE_NEW_PROCESS_GROUP is re-exported for child creation.
const CREATE_NEW_PROCESS_GROUP = windows.CREATE_NEW_PROCESS_GROUP

// DETACHED_PROCESS is re-exported so a long-lived background service is not
// tied to the console of the process that launched it.
const DETACHED_PROCESS = windows.DETACHED_PROCESS

// Job is a Windows Job Object configured to kill all assigned processes when
// the last handle is closed. That guarantees children cannot outlive the
// supervisor even on a hard crash.
type Job struct {
	handle windows.Handle
}

// NewKillOnCloseJob creates the containment job.
func NewKillOnCloseJob() (*Job, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(
		h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		_ = windows.CloseHandle(h)
		return nil, err
	}
	return &Job{handle: h}, nil
}

// Assign adds a child process to the containment job.
func (j *Job) Assign(pid uint32) error {
	if j == nil || j.handle == 0 {
		return errors.New("job object is not initialized")
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.AssignProcessToJobObject(j.handle, h)
}

// Close releases the job handle; all children are terminated as a result.
func (j *Job) Close() error {
	if j == nil || j.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(j.handle)
	j.handle = 0
	return err
}

// CreationTime returns a stable process start timestamp used to detect PID
// reuse before signalling or terminating a process.
func CreationTime(pid uint32) (uint64, error) {
	if pid == 0 {
		return 0, errors.New("invalid pid")
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(h)
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return 0, err
	}
	return uint64(creation.LowDateTime) | (uint64(creation.HighDateTime) << 32), nil
}

// SameProcess verifies that a PID still identifies the originally launched process.
func SameProcess(pid uint32, expectedCreation uint64) bool {
	if expectedCreation == 0 {
		return false
	}
	actual, err := CreationTime(pid)
	return err == nil && actual == expectedCreation
}

// SignalGraceful sends CTRL_BREAK to a child created in its own process group.
func SignalGraceful(p *os.Process) error {
	return p.Signal(os.Interrupt)
}
