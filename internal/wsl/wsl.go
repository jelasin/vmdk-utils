package wsl

import (
	"os"
	"strings"
)

func Enabled() bool {
	return os.Getenv("WSL_DISTRO_NAME") != "" || hasWSLKernelMarker()
}

func hasWSLKernelMarker() bool {
	content, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(content)), "microsoft")
}
