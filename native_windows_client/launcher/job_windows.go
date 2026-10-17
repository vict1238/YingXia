// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"
)

var kernel = syscall.NewLazyDLL("kernel32.dll")
var user = syscall.NewLazyDLL("user32.dll")
var createJobObject = kernel.NewProc("CreateJobObjectW")
var setInformationJobObject = kernel.NewProc("SetInformationJobObject")
var assignProcessToJobObject = kernel.NewProc("AssignProcessToJobObject")
var openProcess = kernel.NewProc("OpenProcess")
var closeHandle = kernel.NewProc("CloseHandle")
var messageBox = user.NewProc("MessageBoxW")
var createMutex = kernel.NewProc("CreateMutexW")
var findWindow = user.NewProc("FindWindowW")
var showWindow = user.NewProc("ShowWindow")
var setForegroundWindow = user.NewProc("SetForegroundWindow")

type jobBasicLimits struct {
	PerProcessTime, PerJobTime                   int64
	LimitFlags                                   uint32
	MinimumWorkingSetSize, MaximumWorkingSetSize uintptr
	ActiveProcessLimit                           uint32
	Affinity                                     uintptr
	PriorityClass, SchedulingClass               uint32
}
type jobExtendedLimits struct {
	Basic                                                                        jobBasicLimits
	IOCounters                                                                   [6]uint64
	ProcessMemoryLimit, JobMemoryLimit, PeakProcessMemoryUsed, PeakJobMemoryUsed uintptr
}
type childJob struct{ handle uintptr }

func newChildJob() (childJob, error) {
	handle, _, err := createJobObject.Call(0, 0)
	if handle == 0 {
		return childJob{}, err
	}
	limits := jobExtendedLimits{}
	limits.Basic.LimitFlags = 0x2000
	ok, _, err := setInformationJobObject.Call(handle, 9, uintptr(unsafe.Pointer(&limits)), unsafe.Sizeof(limits))
	if ok == 0 {
		closeHandle.Call(handle)
		return childJob{}, err
	}
	return childJob{handle}, nil
}
func (job *childJob) Close() {
	if job.handle != 0 {
		closeHandle.Call(job.handle)
		job.handle = 0
	}
}
func (job childJob) Assign(pid int) error {
	handle, _, err := openProcess.Call(0x0101, 0, uintptr(pid))
	if handle == 0 {
		return err
	}
	defer closeHandle.Call(handle)
	ok, _, err := assignProcessToJobObject.Call(job.handle, handle)
	if ok == 0 {
		return fmt.Errorf("无法管理原生窗口进程：%w", err)
	}
	return nil
}
func configureChild(command *exec.Cmd) {
	// Suppress console allocation without hiding the GUI's first ShowWindow.
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
}
func showError(text string) {
	message, _ := syscall.UTF16PtrFromString(text)
	title, _ := syscall.UTF16PtrFromString("映匣 · YingXia")
	messageBox.Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x10)
}

func claimLauncher(windowClass string) (func(), bool, error) {
	name, err := syscall.UTF16PtrFromString("Local\\" + windowClass)
	if err != nil {
		return nil, false, err
	}
	handle, _, callErr := createMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return nil, false, callErr
	}
	return func() { closeHandle.Call(handle) }, callErr != syscall.ERROR_ALREADY_EXISTS, nil
}

func restoreExistingWindow(windowClass string) {
	name, _ := syscall.UTF16PtrFromString(windowClass)
	for attempt := 0; attempt < 100; attempt++ {
		hwnd, _, _ := findWindow.Call(uintptr(unsafe.Pointer(name)), 0)
		if hwnd != 0 {
			showWindow.Call(hwnd, 9)
			setForegroundWindow.Call(hwnd)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}
