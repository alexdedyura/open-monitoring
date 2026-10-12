//go:build windows

package metrics

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"open-monitoring/internal/sidecar"
)

// PresentMon's ETW session needs administrator rights, and the app runs as
// invoker on purpose (see the manifest). So when PresentMon is refused, this
// process starts a second copy of its own executable elevated — one UAC
// prompt — in a headless "relay" mode, and that copy runs PresentMon.
//
// The relay exists because UAC cannot hand an elevated process stdout back to
// a non-elevated parent, and because the handle ShellExecuteEx returns cannot
// terminate what it launched. So:
//
//   - the data path is a named pipe this process serves and the relay
//     connects to; PresentMon's stdout is the relay's end of it, directly;
//   - the lifetime is the relay's job: it watches this process and, when it
//     goes, stops PresentMon and its ETW session. PresentMon also sits in a
//     kill-on-close job, so a relay that dies by any other means takes it
//     along instead of leaving an elevated orphan behind.

// FPSRelayArg is the command-line switch main.go answers by running
// RunFPSRelay instead of the application.
const FPSRelayArg = "--fps-relay"

// fpsPipePrefix starts every pipe name; the relay refuses anything else.
const fpsPipePrefix = `\\.\pipe\OpenMonitoring-fps-`

var (
	errDeclined     = errors.New("the UAC prompt was declined")
	errRelayStopped = errors.New("the elevated frame-capture helper stopped")
)

// runElevated keeps the elevated relay going for the rest of the run. When the
// prompt is declined, or the relay stops, it waits for the user to ask again
// from Settings rather than raising another prompt on its own: a dialog that
// comes back after being dismissed is not a question any more.
func (f *FPSSource) runElevated() {
	for {
		select { // a click from before this attempt is not a request for the next
		case <-f.retry:
		default:
		}
		f.mu.Lock()
		f.lastErr, f.canElevate = "fps.err.pending", false
		f.mu.Unlock()

		err := f.streamElevated()
		if f.stopped() {
			return
		}

		f.mu.Lock()
		f.canElevate = true
		if errors.Is(err, errDeclined) {
			f.lastErr = "fps.err.declined"
		} else {
			f.lastErr = "fps.err.relay"
		}
		f.mu.Unlock()

		select {
		case <-f.retry:
		case <-f.done:
			return
		}
	}
}

// streamElevated serves a pipe, launches the relay into it and consumes what
// arrives until the relay goes away.
func (f *FPSSource) streamElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	// The pid and a random suffix make the name unguessable ahead of time, and
	// FIRST_PIPE_INSTANCE makes creation fail rather than join a pipe someone
	// else created under it first.
	var nonce [8]byte
	rand.Read(nonce[:])
	name := fmt.Sprintf("%s%d-%x", fpsPipePrefix, os.Getpid(), nonce)
	pipe, err := windows.CreateNamedPipe(windows.StringToUTF16Ptr(name),
		windows.PIPE_ACCESS_INBOUND|windows.FILE_FLAG_FIRST_PIPE_INSTANCE,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS,
		1, 0, 256*1024, 0, nil)
	if err != nil {
		return err
	}
	r := os.NewFile(uintptr(pipe), name)
	defer r.Close()

	relay, err := shellExecuteRunAs(exe, fmt.Sprintf("%s %s %d", FPSRelayArg, name, os.Getpid()))
	if err != nil {
		return err
	}
	relayGone := make(chan struct{})
	go func() {
		windows.WaitForSingleObject(relay, windows.INFINITE)
		windows.CloseHandle(relay)
		close(relayGone)
	}()

	connected := make(chan error, 1)
	go func() {
		err := windows.ConnectNamedPipe(pipe, nil)
		if errors.Is(err, windows.ERROR_PIPE_CONNECTED) {
			err = nil // the relay was quicker than this call; still connected
		}
		connected <- err
	}()

	select {
	case err := <-connected:
		if err != nil {
			return err
		}
	case <-relayGone:
		releasePipe(name, connected)
		return errRelayStopped
	case <-f.done:
		releasePipe(name, connected)
		return nil
	}

	f.mu.Lock()
	f.running, f.lastErr = true, ""
	f.mu.Unlock()

	f.consume(r)
	f.streamEnded()
	return errRelayStopped
}

// releasePipe frees a ConnectNamedPipe still waiting for a relay that will
// never connect, by connecting to the pipe from here and hanging up.
func releasePipe(name string, connected <-chan error) {
	if c, err := os.OpenFile(name, os.O_WRONLY, 0); err == nil {
		c.Close()
	}
	<-connected
}

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     windows.Handle
}

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	swHide                = 0
)

var procShellExecuteExW = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// shellExecuteRunAs starts file elevated and returns its process handle — good
// for waiting on, not for terminating. It blocks while the UAC prompt is up.
func shellExecuteRunAs(file, params string) (windows.Handle, error) {
	// ShellExecuteEx may hand the launch to shell extensions, which need COM on
	// the calling thread — so pin one and initialise it.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err == nil || err == syscall.Errno(1) { // S_FALSE: already initialised
		defer windows.CoUninitialize()
	}

	info := shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess | seeMaskNoAsync,
		lpVerb:       windows.StringToUTF16Ptr("runas"),
		lpFile:       windows.StringToUTF16Ptr(file),
		lpParameters: windows.StringToUTF16Ptr(params),
		nShow:        swHide,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))

	ok, _, err := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return 0, errDeclined
		}
		return 0, err
	}
	if info.hProcess == 0 {
		return 0, errRelayStopped
	}
	return info.hProcess, nil
}

// RunFPSRelay is the elevated side: `open-monitoring.exe --fps-relay <pipe>
// <pid>`. It runs PresentMon with its output going straight into the pipe,
// restarts it if it exits, and stops it — ETW session included — once the
// process that asked for it is gone. Returns the exit code.
func RunFPSRelay(args []string) int {
	if len(args) != 2 || !strings.HasPrefix(args[0], fpsPipePrefix) {
		return 2
	}
	parentPID, err := strconv.ParseUint(args[1], 10, 32)
	if err != nil {
		return 2
	}
	parent, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(parentPID))
	if err != nil {
		return 1
	}
	out, err := os.OpenFile(args[0], os.O_WRONLY, 0)
	if err != nil {
		return 1
	}
	defer out.Close()

	path, err := sidecar.Path(sidecar.PresentMon)
	if err != nil {
		return 1
	}
	job, err := killOnCloseJob()
	if err != nil {
		return 1
	}
	defer windows.CloseHandle(job)

	parentGone := make(chan struct{})
	go func() {
		windows.WaitForSingleObject(parent, windows.INFINITE)
		close(parentGone)
	}()

	// PresentMon has no reason to exit while it is wanted. One that does is
	// restarted; one that keeps exiting straight away is given up on, and the
	// app learns of it from the pipe closing.
	const maxQuickExits = 3
	quickExits := 0
	for {
		started := time.Now()
		cmd := exec.Command(path, presentMonArgs()...)
		hideWindow(cmd)
		cmd.Stdout = out
		if err := cmd.Start(); err != nil {
			return 1
		}
		if h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
			false, uint32(cmd.Process.Pid)); err == nil {
			windows.AssignProcessToJobObject(job, h)
			windows.CloseHandle(h)
		}
		exited := make(chan struct{})
		go func() {
			cmd.Wait()
			close(exited)
		}()

		select {
		case <-parentGone:
			cmd.Process.Kill()
			<-exited
			endFPSSession(path)
			return 0
		case <-exited:
		}

		if time.Since(started) < 10*time.Second {
			if quickExits++; quickExits >= maxQuickExits {
				return 1
			}
		} else {
			quickExits = 0
		}
		select {
		case <-parentGone:
			endFPSSession(path)
			return 0
		case <-time.After(5 * time.Second):
		}
	}
}

// killOnCloseJob makes a job object whose processes die with its last handle,
// which is held by the relay alone.
func killOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

// endFPSSession stops the ETW session a killed PresentMon leaves behind. Only
// an elevated process may, which is why the relay does it rather than the app.
func endFPSSession(path string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--terminate_existing_session", "--session_name", fpsSessionName)
	hideWindow(cmd)
	cmd.Run()
}
