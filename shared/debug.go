package shared

import "log"

// debug enables the collectors' tracing: the per-poll lines that report which
// cards, drives and sensor chips were found and why a device was skipped. It is
// off by default because the app's normal output is its window, and a handful of
// lines per card every 5 seconds would bury any genuine warning.
//
// It is a package variable rather than a flag so the collectors do not each have
// to parse one; main.go wires it to -debug.
var debug bool

// SetDebug turns the collectors' tracing on or off.
func SetDebug(on bool) {
	debug = on
}

// DebugEnabled reports whether tracing is on.
func DebugEnabled() bool {
	return debug
}

// Debugf logs through the standard logger only when -debug is set. The format
// string should not end in a newline: log.Printf adds one when it is missing.
func Debugf(format string, args ...any) {
	if !debug {
		return
	}
	log.Printf(format, args...)
}
