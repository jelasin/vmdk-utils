package mount

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jelasin/vmdk-utils/internal/runtime"
)

func Mount(device, mountpoint string, readOnly bool) error {
	args := []string{}
	if readOnly {
		args = append(args, "-o", "ro")
	}
	args = append(args, device, mountpoint)
	if _, err := runtime.RunCombined("mount", args...); err != nil {
		if readOnly {
			if retryArgs, ok := recoverySafeMountArgs(device, mountpoint); ok {
				if _, retryErr := runtime.RunCombined("mount", retryArgs...); retryErr == nil {
					return nil
				}
			}
		}
		return fmt.Errorf("mount %s to %s: %w", device, mountpoint, err)
	}
	return nil
}

func recoverySafeMountArgs(device, mountpoint string) ([]string, bool) {
	fstype, err := filesystemType(device)
	if err != nil {
		return nil, false
	}
	switch fstype {
	case "ext3", "ext4":
		return []string{"-t", fstype, "-o", "ro,noload", device, mountpoint}, true
	case "xfs":
		return []string{"-t", fstype, "-o", "ro,norecovery", device, mountpoint}, true
	default:
		return nil, false
	}
}

func filesystemType(device string) (string, error) {
	output, err := runtime.RunCombined("lsblk", "-n", "-o", "FSTYPE", device)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line, nil
		}
	}
	return "", fmt.Errorf("filesystem type unavailable for %s", device)
}

func Umount(mountpoint string) error {
	if _, err := runtime.RunCombined("umount", mountpoint); err != nil {
		return fmt.Errorf("umount %s: %w", mountpoint, err)
	}
	return nil
}

func ResolveGuestPath(mountpoint, guestPath string) string {
	trimmed := strings.TrimPrefix(guestPath, "/")
	if trimmed == "" {
		return mountpoint
	}
	return filepath.Join(mountpoint, trimmed)
}

func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func CopyOut(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	if info.IsDir() {
		return copyDir(source, destination)
	}
	return copyFile(source, destination)
}

func CopyIn(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	if info.IsDir() {
		return copyDir(source, destination)
	}
	return copyFile(source, destination)
}

func copyFile(source, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	mode := os.FileMode(0o644)
	if info, err := in.Stat(); err == nil {
		mode = info.Mode()
	}
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("open destination: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy file: %w", err)
	}
	return nil
}

func copyDir(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat source dir: %w", err)
	}
	if err := os.MkdirAll(destination, info.Mode()); err != nil {
		return fmt.Errorf("create destination dir: %w", err)
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read source dir: %w", err)
	}
	for _, entry := range entries {
		src := filepath.Join(source, entry.Name())
		dst := filepath.Join(destination, entry.Name())
		if entry.IsDir() {
			if err := copyDir(src, dst); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(src, dst); err != nil {
			return err
		}
	}
	return nil
}
