package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// tmuxHasSession reports whether a tmux session exists.
func tmuxHasSession(sess string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", sess)
	return cmd.Run() == nil
}

// tmuxSend sends a line to a tmux session.
func tmuxSend(sess, line string) {
	exec.Command("tmux", "send-keys", "-t", sess, line, "Enter").Run()
}

// tmuxAttach attaches the current terminal to a session (replaces process).
func tmuxAttach(sess string) {
	path, err := exec.LookPath("tmux")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mc:", err)
		os.Exit(1)
	}
	if err := syscall.Exec(path, []string{"tmux", "attach-session", "-t", sess}, os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, "mc:", err)
		os.Exit(1)
	}
}

// serverRunning reports whether a process matching the jar name is running.
func serverRunning(jarname string) bool {
	cmd := exec.Command("pgrep", "-f", jarname)
	return cmd.Run() == nil
}

// javaPidsForDir returns the PIDs of processes running the given jar whose
// working directory is dir. Matching by cwd prevents killing a different
// server that happens to share the same jar filename.
func javaPidsForDir(dir, jarname string) []string {
	out, err := exec.Command("pgrep", "-f", jarname).Output()
	if err != nil {
		return nil
	}
	realDir, _ := filepath.EvalSymlinks(dir)
	var pids []string
	for _, pid := range strings.Fields(string(out)) {
		cwd, err := os.Readlink("/proc/" + pid + "/cwd")
		if err != nil {
			continue
		}
		realCwd, _ := filepath.EvalSymlinks(cwd)
		if realCwd == realDir {
			pids = append(pids, pid)
		}
	}
	return pids
}

// waitStop polls tmux until the session disappears. On each 30s timeout it
// prompts for a force-kill. If declined, it retries the wait up to
// maxRetries times; after the retries are exhausted it stops waiting and
// lists the still-alive tmux session and/or java PID for manual handling.
// Pressing any key during a wait jumps straight to the force-kill prompt.
func waitStop(srv, session, jarname string, timeout, maxRetries int) {
	interactive := isTTY(os.Stdin)
	if interactive {
		enableCbreak()
		defer restoreTTY()
	}
	retries := 0
	for {
		// Wait up to `timeout` seconds, but bail to the prompt immediately if
		// a key is pressed (only when interactive).
		elapsed := 0
		for elapsed < timeout && tmuxHasSession(session) {
			if interactive && readKey(2*time.Second) != 0 {
				break // key pressed -> jump to prompt
			}
			if !interactive {
				time.Sleep(2 * time.Second)
			}
			elapsed += 2
		}
		if !tmuxHasSession(session) {
			return
		}

		if retries == 0 {
			fmt.Printf("Server did not stop after %d seconds.\n", timeout)
		}
		confirm := false
		if interactive {
			fmt.Print("Force kill? [y/N] ")
			confirm = askYN()
		}
		if confirm {
			exec.Command("pkill", "-9", "-f", "tmux new-session -d -s "+session).Run()
			exec.Command("pkill", "-9", "-f", "-jar "+jarname).Run()
			time.Sleep(2 * time.Second)
			return
		}

		retries++
		if retries > maxRetries {
			fmt.Println("Gave up waiting after", retries, "attempts.")
			reportLeftovers(session, jarname)
			return
		}
		fmt.Printf("Retrying graceful stop (attempt %d/%d)...\n", retries, maxRetries)
	}
}

// reportLeftovers prints the surviving tmux session and/or java PID so the
// user can deal with a stuck server manually.
func reportLeftovers(session, jarname string) {
	if tmuxHasSession(session) {
		fmt.Printf("Still running tmux session: %s\n", session)
	} else {
		fmt.Println("No tmux session found.")
	}
	cmd := exec.Command("pgrep", "-f", jarname)
	if out, err := cmd.Output(); err == nil {
		if pids := strings.Fields(string(out)); len(pids) > 0 {
			fmt.Printf("Still running java PID(s): %s\n", strings.Join(pids, " "))
			return
		}
	}
	fmt.Println("No matching java process found.")
}

// ----- tty / keyboard helpers -----

var savedTermios *unix.Termios

func isTTY(f *os.File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), unix.TCGETS)
	return err == nil
}

// enableCbreak puts stdin into cbreak mode (ICANON|ECHO off) so a single
// keypress becomes readable without an Enter. Restore with restoreTTY.
func enableCbreak() {
	fd := int(os.Stdin.Fd())
	t, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return
	}
	c := *t
	c.Lflag &^= unix.ICANON | unix.ECHO
	c.Cc[unix.VMIN] = 0
	c.Cc[unix.VTIME] = 0
	unix.IoctlSetTermios(fd, unix.TCSETS, &c)
	savedTermios = t
}

func restoreTTY() {
	if savedTermios == nil {
		return
	}
	unix.IoctlSetTermios(int(os.Stdin.Fd()), unix.TCSETS, savedTermios)
	savedTermios = nil
}

// readKey blocks up to the given timeout for a single keypress on stdin.
// Returns the byte read, or 0 if timed out with no input. Consumes the key.
// Uses poll(2) so it's interruptible and works in cbreak mode.
func readKey(timeout time.Duration) byte {
	fds := []unix.PollFd{{Fd: int32(os.Stdin.Fd()), Events: unix.POLLIN}}
	ms := int(timeout / time.Millisecond)
	n, err := unix.Poll(fds, ms)
	if err != nil || n == 0 {
		return 0
	}
	if fds[0].Revents&unix.POLLIN == 0 {
		return 0
	}
	var b [1]byte
	if _, err := os.Stdin.Read(b[:]); err != nil {
		return 0
	}
	return b[0]
}

// askYN reads a single char for a y/N confirmation and cleans up any trailing
// buffered input (Enter, etc.) so it doesn't leak into later prompts.
func askYN() bool {
	for {
		c := readKey(200 * time.Millisecond)
		if c == 0 {
			continue
		}
		// Drain the rest of the line (Enter, CR, etc.).
		drainInput()
		return c == 'y' || c == 'Y'
	}
}

// drainInput consumes any currently-buffered stdin bytes without blocking.
func drainInput() {
	fds := []unix.PollFd{{Fd: int32(os.Stdin.Fd()), Events: unix.POLLIN}}
	if n, _ := unix.Poll(fds, 0); n > 0 {
		buf := make([]byte, 4096)
		os.Stdin.Read(buf)
	}
}
