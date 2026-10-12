//go:build !windows

package main

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		cmd.Process.Signal(syscall.SIGTERM)
		go func(p *os.Process) {
			// 크롬이 스스로 정리할 시간을 조금 준 뒤 강제로 닫아요
			exec.Command("/bin/sleep", "2").Run()
			syscall.Kill(-p.Pid, syscall.SIGKILL)
			p.Kill()
		}(cmd.Process)
	}
}

func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func notifyOS(title, msg string) {
	if p := os.Getenv("VALOROAD_NOTIFY_LOG"); p != "" { // 테스트용
		f, _ := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if f != nil {
			f.WriteString(title + " | " + msg + "\n")
			f.Close()
		}
	}
	if runtime.GOOS != "darwin" {
		return
	}
	exec.Command("/usr/bin/osascript", "-e", "on run argv", "-e",
		`display notification (item 2 of argv) with title (item 1 of argv) sound name "Glass"`, "-e", "end run", title, msg).Start()
}
