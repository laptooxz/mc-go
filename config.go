package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// configPath is the future home for an optional mc config override.
func configPath() string {
	return filepath.Join(os.Getenv("HOME"), ".config/mc/config.json")
}
