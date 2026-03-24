package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func RunCombined(name string, args ...string) (string, error) {
	resolved, err := resolveBinary(name)
	if err != nil {
		return "", err
	}

	cmd := exec.Command(resolved, args...)
	cmd.Env = append(os.Environ(), augmentedEnv()...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run %s %s: %w\n%s", resolved, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func resolveBinary(name string) (string, error) {
	runtimePath := filepath.Join(projectRoot(), "runtime", "bin", name)
	if info, err := os.Stat(runtimePath); err == nil && !info.IsDir() {
		return runtimePath, nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("binary %q not found in runtime/bin or PATH", name)
	}
	return path, nil
}

func augmentedEnv() []string {
	var env []string
	runtimeBin := filepath.Join(projectRoot(), "runtime", "bin")
	runtimeLib := filepath.Join(projectRoot(), "runtime", "lib")
	env = append(env, "PATH="+runtimeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := os.Stat(runtimeLib); err == nil {
		current := os.Getenv("LD_LIBRARY_PATH")
		if current == "" {
			env = append(env, "LD_LIBRARY_PATH="+runtimeLib)
		} else {
			env = append(env, "LD_LIBRARY_PATH="+runtimeLib+string(os.PathListSeparator)+current)
		}
	}
	return env
}

func projectRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
