package main

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func soundFor(name string) string {
	sounds := filepath.Join(home(), "mc/library/sounds")
	switch name {
	case "start":
		return filepath.Join(sounds, "block/beacon/activate.ogg")
	case "stop":
		return filepath.Join(sounds, "block/beacon/deactivate.ogg")
	case "ready":
		return filepath.Join(sounds, "random/levelup.ogg")
	case "join":
		return filepath.Join(sounds, "note/pling.ogg")
	case "leave":
		return filepath.Join(sounds, "random/break.ogg")
	case "death":
		return filepath.Join(sounds, "mob/wither/spawn.ogg")
	case "chunky-start":
		return filepath.Join(sounds, "random/orb.ogg")
	case "chunky-finish":
		return filepath.Join(sounds, "item/goat_horn/call0.ogg")
	case "window-attention":
		return filepath.Join(sounds, "item/goat_horn/call1.ogg")
	case "message-new-instant":
		return filepath.Join(sounds, "mob/wither/spawn.ogg")
	default:
		return filepath.Join(sounds, "random/orb.ogg")
	}
}

// hostEnv computes the host GUI session environment variables (mirrors hostenv).
func hostEnv() []string {
	env := map[string]string{}
	runtime := "/run/user/" + itoa(os.Getuid())

	env["XDG_RUNTIME_DIR"] = runtime

	if isSock(runtime + "/pulse/native") {
		env["PULSE_SERVER"] = "unix:" + runtime + "/pulse/native"
	} else if isSock(runtime + "/pipewire-0") {
		env["PULSE_SERVER"] = "unix:" + runtime + "/pipewire-0"
	}
	if isSock(runtime + "/pipewire-0") {
		env["PIPEWIRE_RUNTIME_DIR"] = runtime
	}

	env["DBUS_SESSION_BUS_ADDRESS"] = ""
	if b, err := os.ReadFile(filepath.Join(home(), ".cache/dbus-session")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "DBUS_SESSION_BUS_ADDRESS=") {
				env["DBUS_SESSION_BUS_ADDRESS"] = strings.TrimPrefix(line, "DBUS_SESSION_BUS_ADDRESS=")
				break
			}
		}
	}
	if env["DBUS_SESSION_BUS_ADDRESS"] == "" {
		if isSock(runtime + "/bus") {
			env["DBUS_SESSION_BUS_ADDRESS"] = "unix:path=" + runtime + "/bus"
		}
	}
	if env["DBUS_SESSION_BUS_ADDRESS"] == "" {
		if matches, _ := filepath.Glob(runtime + "/dbus-*"); len(matches) > 0 {
			for _, m := range matches {
				if isSock(m) {
					env["DBUS_SESSION_BUS_ADDRESS"] = "unix:path=" + m
					break
				}
			}
		}
	}

	if matches, _ := filepath.Glob(runtime + "/wayland-*"); len(matches) > 0 {
		for _, m := range matches {
			if isSock(m) {
				env["WAYLAND_DISPLAY"] = strings.TrimPrefix(m, runtime+"/")
				env["XDG_SESSION_TYPE"] = "wayland"
				break
			}
		}
	}

	if matches, _ := filepath.Glob(runtime + "/sway-ipc.*.sock"); len(matches) > 0 {
		for _, m := range matches {
			if isSock(m) {
				env["SWAYSOCK"] = m
				break
			}
		}
	}

	if _, err := os.Stat("/tmp/.X11-unix/X0"); err == nil {
		env["DISPLAY"] = ":0"
	}

	// Merge onto current environment, only overriding known vars.
	merged := os.Environ()
	seen := map[string]string{}
	for _, kv := range merged {
		k := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			k = kv[:i]
		}
		seen[k] = kv
	}
	// We rebuild: start from a copy that includes the host vars overriding.
	return mergeEnv(merged, env)
}

func mergeEnv(base []string, over map[string]string) []string {
	// base is a list of K=V. Replace keys in over.
	keys := map[string]bool{}
	for k := range over {
		keys[k] = true
	}
	for i, kv := range base {
		k := kv
		if j := strings.IndexByte(kv, '='); j >= 0 {
			k = kv[:j]
		}
		if keys[k] {
			base[i] = k + "=" + over[k]
			delete(keys, k)
		}
	}
	for k := range keys {
		base = append(base, k+"="+over[k])
	}
	return base
}

func isSock(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode()&os.ModeSocket != 0
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// runInHostEnv invokes a command with the host GUI env; runs asynchronously.
func runInHostEnv(argv ...string) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = hostEnv()
	cmd.Start()
}

// ntfyPush sends a notification to the local ntfy server (mirrors ntfy-push).
func ntfyPush(topic, title, msg string, priority int) {
	token := ""
	if b, err := os.ReadFile("/etc/sysmon/tokens/ntfy.sh"); err == nil {
		s := strings.TrimSpace(string(b))
		if strings.HasPrefix(s, "NTFY_TOKEN=") {
			token = strings.Trim(strings.TrimPrefix(s, "NTFY_TOKEN="), "\"'")
		}
	}
	body := strings.NewReader(msg)
	req, err := http.NewRequest("POST", "http://127.0.0.1:8989/"+topic, body)
	if err != nil {
		return
	}
	req.Header.Set("Title", title)
	req.Header.Set("Priority", itoa(priority))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{}
	go client.Do(req)
}

// notify reimplements _mc_notify: desktop notify-send, optional sound, ntfy.
func notify(title, msg, soundName string) {
	// Desktop notification
	if _, err := exec.LookPath("notify-send"); err == nil {
		runInHostEnv("notify-send", "-a", "laptooMC", "-i", "minecraft", title, msg)
	}

	// Sound
	if soundName != "" {
		sound := soundFor(soundName)
		if _, err := os.Stat(sound); err == nil {
			if _, err := exec.LookPath("paplay"); err == nil {
				runInHostEnv("paplay", sound)
			}
		}
	}

	// ntfy
	ntfyPush("mc", title, msg, 3)
}
