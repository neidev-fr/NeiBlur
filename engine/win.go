package engine

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"

	"golang.org/x/sys/windows"
)

const (
	createNoWindow         = 0x08000000
	belowNormalPriority    = 0x00004000
	processSuspendResume   = 0x0800
	esContinuous           = 0x80000000
	esSystemRequired       = 0x00000001
	pipeBufferSize         = 4 << 20 // 4 Mo : moins d'allers-retours noyau entre VSPipe et FFmpeg
)

var (
	ntdll              = windows.NewLazySystemDLL("ntdll.dll")
	procNtSuspend      = ntdll.NewProc("NtSuspendProcess")
	procNtResume       = ntdll.NewProc("NtResumeProcess")
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procSetExecState   = kernel32.NewProc("SetThreadExecutionState")
)

// hiddenProc : pas de fenêtre console, priorité basse optionnelle (le PC reste réactif pendant un rendu).
func hiddenProc(lowPriority bool) *syscall.SysProcAttr {
	flags := uint32(createNoWindow)
	if lowPriority {
		flags |= belowNormalPriority
	}
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags}
}

// bigPipe crée un tube anonyme avec un gros tampon (os.Pipe utilise la taille par défaut).
func bigPipe() (r, w *os.File, err error) {
	var rh, wh windows.Handle
	if err = windows.CreatePipe(&rh, &wh, nil, pipeBufferSize); err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(rh), "pipe-r"), os.NewFile(uintptr(wh), "pipe-w"), nil
}

func setSuspended(p *os.Process, suspend bool) error {
	if p == nil {
		return nil
	}
	h, err := windows.OpenProcess(processSuspendResume, false, uint32(p.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	proc := procNtResume
	if suspend {
		proc = procNtSuspend
	}
	r, _, _ := proc.Call(uintptr(h))
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

// KeepAwake empêche la mise en veille tant que le canal n'est pas fermé.
// L'état d'exécution est lié au thread : on verrouille une goroutine sur son thread OS.
func KeepAwake(stop <-chan struct{}) {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		procSetExecState.Call(uintptr(esContinuous | esSystemRequired))
		<-stop
		procSetExecState.Call(uintptr(esContinuous))
	}()
}

// RevealInExplorer ouvre l'Explorateur avec le fichier sélectionné.
func RevealInExplorer(path string) error {
	cmd := exec.Command("explorer.exe", "/select,", path)
	return cmd.Start()
}

func OpenPath(path string) error {
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", path).Start()
}
