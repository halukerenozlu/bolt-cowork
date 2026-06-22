package views

import (
	"fmt"
	"os/exec"
	"runtime"
)

func openURL(url string) error {
	cmd := browserCommand(runtime.GOOS, url)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open URL: %w", err)
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

func browserCommand(goos, url string) *exec.Cmd {
	switch goos {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		return exec.Command("open", url)
	default:
		return exec.Command("xdg-open", url)
	}
}
