//go:build !windows

package main

import (
	"os/exec"
	"runtime"
	"syscall"
)

func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// detachSelf moves this process into its own session, so it keeps running when the launcher ends.
func detachSelf() { syscall.Setsid() }

func hideWindow(cmd *exec.Cmd) {}

// openURL opens a web page or file with its default app.
func openURL(u string) {
	if runtime.GOOS == "darwin" {
		exec.Command("/usr/bin/open", u).Start()
	}
}
