package shared

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// RunShell executes command through the system shell and returns its trimmed
// stdout. It returns an empty string when the command fails, so callers can
// treat "no output" and "could not run" the same way.
func RunShell(command string) string {
	output, err := exec.Command("sh", "-c", command).Output()
	if err != nil {
		log.Printf("Error executing command %q: %v\n", command, err)
		return ""
	}
	return strings.TrimSpace(string(output))
}

// FindFirstSubpath returns the first path matching pattern directly beneath
// basePath, or an empty string when nothing matches. It is used to locate the
// hwmon directory of a DRM card (e.g. "class/drm/card0/device/hwmon", "hwmon*").
func FindFirstSubpath(basePath, pattern string) string {
	matches, _ := filepath.Glob(filepath.Join(basePath, pattern))
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// ReadSysFile reads a sysfs file and reports whether it could be read. Callers
// that need to distinguish "sensor absent" from "sensor reads zero" must use
// this rather than ReadSysValue.
func ReadSysFile(path string) (string, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(content)), true
}

// ReadSysValue reads a sysfs file and returns its trimmed contents, logging and
// returning "0" when the file cannot be read. Prefer ReadSysFile when the
// absence of the file should suppress the metric entirely.
func ReadSysValue(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		log.Printf("Error reading %s: %v\n", path, err)
		return "0"
	}
	return strings.TrimSpace(string(content))
}

// ParseFrequencyMHz converts a raw sysfs frequency value in Hz to MHz. It
// returns 0 when the value cannot be parsed.
func ParseFrequencyMHz(value string) float64 {
	freq, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		log.Printf("Error parsing frequency value %q: %v\n", value, err)
		return 0
	}
	return freq / 1e6
}

// ParsePower converts a raw sysfs power value to watts. Sysfs reports power in
// microwatts, so callers normally pass divisor 1000000. Negative or unparsable
// values yield 0.
func ParsePower(raw string, divisor int) float64 {
	val, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || val < 0 {
		return 0
	}
	return float64(val / divisor)
}
