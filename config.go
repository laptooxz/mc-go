package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func jsonUnmarshal(b []byte, v interface{}) error {
	return json.Unmarshal(b, v)
}

// mcArgsLine returns the canonical java command line with paper.jar as the
// jar placeholder (mirrors $mcargs from minecraft.fish). The jar name and
// java provider are substituted per-server at launch.
func mcArgsLine() string {
	return "java -Xms2G -Xmx4G -Djava.net.preferIPv4Stack=true -XX:+UseG1GC " +
		"-XX:+ParallelRefProcEnabled -XX:MaxGCPauseMillis=100 " +
		"-XX:+UnlockExperimentalVMOptions -XX:+DisableExplicitGC -XX:+AlwaysPreTouch " +
		"-XX:G1NewSizePercent=30 -XX:G1MaxNewSizePercent=40 -XX:G1HeapRegionSize=8M " +
		"-XX:G1ReservePercent=20 -XX:G1HeapWastePercent=5 -XX:G1MixedGCCountTarget=4 " +
		"-XX:InitiatingHeapOccupancyPercent=15 -XX:G1MixedGCLiveThresholdPercent=90 " +
		"-XX:G1RSetUpdatingPauseTimePercent=5 -XX:SurvivorRatio=32 " +
		"-XX:MaxTenuringThreshold=1 -Dusing.aikars.flags=https://mcflags.emc.gs " +
		"-Daikars.new.flags=true -jar paper.jar --nogui"
}

// defaultJvmFlags is the JVM flags chunk without the leading "java " and
// trailing " -jar paper.jar --nogui" (i.e. just the flags).
func defaultJvmFlags() string {
	line := mcArgsLine()
	line = strings.TrimPrefix(line, "java ")
	if idx := strings.Index(line, " -jar "); idx >= 0 {
		line = line[:idx]
	}
	return strings.TrimSpace(line)
}

// jvmArgsPath returns the per-server args file path (.java_args primary,
// .mc_args / .jvm_args as fallback for reading).
func jvmArgsPath(dir string) string {
	return filepath.Join(dir, ".java_args")
}

func readJvmArgs(dir string) string {
	// try .java_args first, then fallbacks
	for _, name := range []string{".java_args", ".mc_args", ".jvm_args"} {
		p := filepath.Join(dir, name)
		if b, err := os.ReadFile(p); err == nil {
			s := strings.TrimSpace(string(b))
			if s == "" {
				return defaultJvmFlags()
			}
			// if file contains a full command line (has "java " or " -jar "), extract flags
			if strings.HasPrefix(s, "java ") {
				s = strings.TrimPrefix(s, "java ")
			}
			if idx := strings.Index(s, " -jar "); idx >= 0 {
				s = s[:idx]
			}
			// also handle single-line with newlines collapsed
			s = strings.Join(strings.Fields(s), " ")
			if s == "" {
				return defaultJvmFlags()
			}
			return s
		}
	}
	return defaultJvmFlags()
}

func hasCustomJvmArgs(dir string) bool {
	for _, name := range []string{".java_args", ".mc_args", ".jvm_args"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func writeJvmArgs(dir, flags string) error {
	flags = strings.TrimSpace(strings.Join(strings.Fields(flags), " "))
	if flags == "" {
		return fmt.Errorf("empty flags")
	}
	return os.WriteFile(jvmArgsPath(dir), []byte(flags+"\n"), 0644)
}

func removeJvmArgs(dir string) error {
	for _, name := range []string{".java_args", ".mc_args", ".jvm_args"} {
		_ = os.Remove(filepath.Join(dir, name))
	}
	return nil
}

// parseMem extracts -Xms and -Xmx from a flags string.
func parseMem(flags string) (xms, xmx string) {
	for _, f := range strings.Fields(flags) {
		if strings.HasPrefix(f, "-Xms") {
			xms = f[4:]
		} else if strings.HasPrefix(f, "-Xmx") {
			xmx = f[4:]
		}
	}
	return
}

func updateMem(flags, newXms, newXmx string) string {
	fields := strings.Fields(flags)
	var out []string
	hasXms, hasXmx := false, false
	for _, f := range fields {
		if strings.HasPrefix(f, "-Xms") {
			if newXms != "" {
				out = append(out, "-Xms"+newXms)
				hasXms = true
			}
			continue
		}
		if strings.HasPrefix(f, "-Xmx") {
			if newXmx != "" {
				out = append(out, "-Xmx"+newXmx)
				hasXmx = true
			}
			continue
		}
		out = append(out, f)
	}
	if newXms != "" && !hasXms {
		out = append([]string{"-Xms" + newXms}, out...)
	}
	if newXmx != "" && !hasXmx {
		// insert after Xms if present
		insertAt := 1
		if hasXms {
			insertAt = 1
		} else {
			insertAt = 0
		}
		if insertAt >= len(out) {
			out = append(out, "-Xmx"+newXmx)
		} else {
			tmp := make([]string, 0, len(out)+1)
			tmp = append(tmp, out[:insertAt]...)
			tmp = append(tmp, "-Xmx"+newXmx)
			tmp = append(tmp, out[insertAt:]...)
			out = tmp
		}
	}
	return strings.Join(out, " ")
}

// configPath is the future home for an optional mc config override.
func configPath() string {
	return filepath.Join(os.Getenv("HOME"), ".config/mc/config.json")
}
