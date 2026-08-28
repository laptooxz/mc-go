package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const botPrefix = "mcbot"

func botsDir() string { return filepath.Join(home(), "mc", "bot", ".bots") }

func botSessName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return botPrefix + "-" + b.String()
}

// kickBotsForPort kicks any running bots whose state file port matches.
// Mirrors _mc_kick_bots.
func kickBotsForPort(dir string) {
	port := serverPort(dir)
	if port == "" {
		return
	}
	entries, err := os.ReadDir(botsDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		p := filepath.Join(botsDir(), e.Name())
		b, err := os.ReadFile(p)
		if err != nil || e.IsDir() {
			continue
		}
		fields := strings.Fields(string(b))
		if len(fields) == 0 {
			continue
		}
		if fields[0] == port {
			kickBot(e.Name())
		}
	}
}

// kickBot kicks a named bot (kills its tmux session + removes state).
func kickBot(username string) bool {
	for _, sess := range []string{username, username + "-b"} {
		if tmuxHasSession(botSessName(sess)) {
			execTmux("kill-session", "-t", botSessName(sess))
			os.Remove(filepath.Join(botsDir(), username))
			fmt.Printf("kicked '%s'\n", username)
			return true
		}
	}
	os.Remove(filepath.Join(botsDir(), username))
	return false
}

func execTmux(args ...string) {
	exec.Command("tmux", args...).Run()
}
