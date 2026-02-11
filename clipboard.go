package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// clipboardCmd returns the clipboard command for the current OS, or empty if unavailable.
func clipboardCmd() (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "pbcopy", nil
	case "linux":
		// Try wayland first, then X11
		if _, err := exec.LookPath("wl-copy"); err == nil {
			return "wl-copy", nil
		}
		if _, err := exec.LookPath("xclip"); err == nil {
			return "xclip", []string{"-selection", "clipboard"}
		}
		if _, err := exec.LookPath("xsel"); err == nil {
			return "xsel", []string{"--clipboard", "--input"}
		}
		return "", nil
	case "windows":
		return "clip.exe", nil
	default:
		return "", nil
	}
}

// CopyToClipboard copies text to the system clipboard.
// Returns nil if no clipboard tool is available (silent degradation).
func CopyToClipboard(text string) error {
	name, args := clipboardCmd()
	if name == "" {
		return nil // no clipboard available, not an error
	}
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clipboard copy failed: %w", err)
	}
	return nil
}

// HasClipboard returns true if a clipboard tool is available.
func HasClipboard() bool {
	name, _ := clipboardCmd()
	return name != ""
}
