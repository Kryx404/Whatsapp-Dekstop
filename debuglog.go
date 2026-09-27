package main

// Optional internal diagnostics. Enabled with WA_DESK_DEBUG=1; everything is a
// no-op otherwise. Logs stay on this machine (profile dir / wa_debug.log) and
// are never shipped anywhere.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	debugLogMu     sync.Mutex
	debugLogFile   *os.File
	debugStartedAt = time.Now()
)

// debugMarkerPath is the opt-in file that enables diagnostics without an
// environment variable. A Finder/Dock launch cannot pass WA_DESK_DEBUG, and
// asking a user to run the binary from a terminal is a non-starter, so an empty
// file in the profile directory turns the same logging on; deleting it turns it
// off again. Nothing is uploaded either way.
func debugMarkerPath() string {
	return filepath.Join(filepath.Dir(debugLogPath()), "enable_diag")
}

func debugEnabled() bool {
	if os.Getenv("WA_DESK_DEBUG") == "1" {
		return true
	}
	_, err := os.Stat(debugMarkerPath())
	return err == nil
}

// diagLogFromPage records a page-side diagnostic (drag & drop, document
// preview, account switch) into the same local log. Gated by debugEnabled so
// the steady-state cost for normal users is a single stat call.
func diagLogFromPage(kind, detail string) bool {
	if !debugEnabled() {
		return false
	}
	kind = strings.TrimSpace(strings.ReplaceAll(kind, "\n", " "))
	detail = strings.TrimSpace(strings.ReplaceAll(detail, "\n", " "))
	if len(kind) > 40 {
		kind = kind[:40]
	}
	if len(detail) > 300 {
		detail = detail[:300]
	}
	cacheDebugLog("page %s: %s", kind, detail)
	return true
}

func debugLogPath() string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDirOrDot(), "Library", "Application Support", "WhatsAppDesk", "wa_debug.log")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "WhatsAppDesk", "wa_debug.log")
	default:
		return filepath.Join(homeDirOrDot(), ".config", "whatsapp-desk", "wa_debug.log")
	}
}

func homeDirOrDot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

func cacheDebugLog(format string, args ...interface{}) {
	if !debugEnabled() {
		return
	}
	// A test run must never append to the real profile log. It resolves to the
	// same path, so `go test` used to interleave its own lines (e.g. an update
	// check with a fixture version) into the log a user is asked to read.
	if testing.Testing() {
		return
	}
	debugLogMu.Lock()
	defer debugLogMu.Unlock()
	if debugLogFile == nil {
		_ = os.MkdirAll(filepath.Dir(debugLogPath()), 0755)
		f, err := os.OpenFile(debugLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		debugLogFile = f
	}
	fmt.Fprintf(debugLogFile, "[%s +%4.1fs] %s\n",
		time.Now().Format("15:04:05"), time.Since(debugStartedAt).Seconds(),
		fmt.Sprintf(format, args...))
}

// debugLogProcessStats writes the current process RSS plus, on darwin, the
// resident size of the WebKit child processes. Light: one `ps` invocation.
func debugLogProcessStats(phase string) {
	if !debugEnabled() {
		return
	}
	out, err := os.ReadFile("/proc/self/statm")
	rss := "n/a"
	if err == nil && len(out) > 0 {
		var total, resident uint64
		if _, err := fmt.Sscanf(string(out), "%d %d", &total, &resident); err == nil {
			rss = fmt.Sprintf("%dMB", resident*uint64(pageSize())/1024/1024)
		}
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		rss = fmt.Sprintf("%dMB", approxSelfRSS())
	}
	cacheDebugLog("stats phase=%s rss=%s goroutines=%d", phase, rss, runtime.NumGoroutine())
}

func pageSize() int {
	return 4096
}

func approxSelfRSS() int64 {
	// Cheap, portable-enough approximation for diagnostics only.
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return int64(m.Sys / 1024 / 1024)
}
