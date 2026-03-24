package runtime

import (
	"fmt"
	"os/exec"
	"strings"
)

func RunCombined(name string, args ...string) (string, error) {
	resolved, err := resolveBinary(name)
	if err != nil {
		return "", err
	}

	cmd := exec.Command(resolved, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run %s %s: %w\n%s", resolved, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func resolveBinary(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("binary %q not found in PATH", name)
	}
	return path, nil
}
