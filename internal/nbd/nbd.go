package nbd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jelasin/vmdk-utils/internal/runtime"
)

func FindFreeDevice() (string, error) {
	matches, err := listDevices()
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		if err := loadModule(); err != nil {
			return "", err
		}
		matches, err = listDevices()
		if err != nil {
			return "", err
		}
		if len(matches) == 0 {
			return "", fmt.Errorf("no /dev/nbdX devices found after loading nbd module")
		}
	}

	for _, match := range matches {
		name := filepath.Base(match)
		ready, _, err := readyToAttach("/dev/" + name)
		if err != nil {
			continue
		}
		if ready {
			return "/dev/" + name, nil
		}
	}

	return "", fmt.Errorf("no free nbd devices available")
}

func listDevices() ([]string, error) {
	matches, err := filepath.Glob("/sys/class/block/nbd*")
	if err != nil {
		return nil, fmt.Errorf("glob nbd devices: %w", err)
	}
	devices := make([]string, 0, len(matches))
	for _, match := range matches {
		name := filepath.Base(match)
		if !isWholeDevice(name) {
			continue
		}
		devices = append(devices, match)
	}
	return devices, nil
}

func isWholeDevice(name string) bool {
	if !strings.HasPrefix(name, "nbd") {
		return false
	}
	if len(name) == len("nbd") {
		return false
	}
	for _, r := range name[len("nbd"):] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func loadModule() error {
	if _, err := runtime.RunCombined("modprobe", "nbd", "max_part=16"); err != nil {
		return fmt.Errorf("load nbd module: %w", err)
	}
	return nil
}

func Attach(image, device string, readOnly bool) error {
	if err := WaitForReadyToAttach(device); err != nil {
		return err
	}
	args := []string{"--connect", device}
	if readOnly {
		args = append(args, "--read-only")
	}
	args = append(args, image)
	if _, err := runtime.RunCombined("qemu-nbd", args...); err != nil {
		return fmt.Errorf("qemu-nbd attach: %w", err)
	}
	if _, err := runtime.RunCombined("partprobe", device); err != nil {
		return fmt.Errorf("partprobe %s: %w", device, err)
	}
	_ = waitForPartitionScan(device)
	return nil
}

func waitForPartitionScan(device string) error {
	deadline := time.Now().Add(10 * time.Second)
	lastSnapshot := ""
	stableCount := 0

	for time.Now().Before(deadline) {
		matches, err := filepath.Glob(device + "p*")
		if err != nil {
			return fmt.Errorf("glob partitions for %s: %w", device, err)
		}
		snapshot := strings.Join(matches, "\n")
		if snapshot != "" && snapshot == lastSnapshot {
			stableCount++
			if stableCount >= 2 {
				return nil
			}
		} else {
			stableCount = 0
			lastSnapshot = snapshot
		}
		time.Sleep(250 * time.Millisecond)
	}

	return nil
}

func Detach(device string) error {
	if _, err := runtime.RunCombined("qemu-nbd", "--disconnect", device); err != nil {
		return fmt.Errorf("qemu-nbd detach: %w", err)
	}
	if err := WaitForReadyToAttach(device); err != nil {
		return fmt.Errorf("wait for nbd detach: %w", err)
	}
	return nil
}

func HasActiveBackend(device string) bool {
	pid, err := os.ReadFile(filepath.Join("/sys/class/block", filepath.Base(device), "pid"))
	return err == nil && strings.TrimSpace(string(pid)) != ""
}

func WaitForReadyToAttach(device string) error {
	deadline := time.Now().Add(10 * time.Second)
	lastReason := "not ready"
	for time.Now().Before(deadline) {
		ready, reason, err := readyToAttach(device)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		if reason != "" {
			lastReason = reason
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("%s is not ready for attach: %s", device, lastReason)
}

func readyToAttach(device string) (bool, string, error) {
	name := filepath.Base(device)
	sysPath := filepath.Join("/sys/class/block", name)
	if _, err := os.Stat(sysPath); err != nil {
		return false, "", fmt.Errorf("stat %s: %w", sysPath, err)
	}

	pid, err := os.ReadFile(filepath.Join(sysPath, "pid"))
	if err == nil && strings.TrimSpace(string(pid)) != "" {
		return false, "qemu-nbd backend is still active", nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, "", fmt.Errorf("read nbd pid for %s: %w", device, err)
	}

	size, err := os.ReadFile(filepath.Join(sysPath, "size"))
	if err != nil {
		return false, "", fmt.Errorf("read nbd size for %s: %w", device, err)
	}
	if strings.TrimSpace(string(size)) != "0" {
		return false, "device still reports non-zero size", nil
	}

	partitions, err := filepath.Glob(device + "p*")
	if err != nil {
		return false, "", fmt.Errorf("glob partitions for %s: %w", device, err)
	}
	if len(partitions) > 0 {
		return false, "partition devices are still present", nil
	}

	return true, "", nil
}

func WaitForPartition(device string, partition int) (string, error) {
	if partition <= 0 {
		return "", fmt.Errorf("invalid partition: %d", partition)
	}
	partitionDevice := device + "p" + strconv.Itoa(partition)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(partitionDevice); err == nil {
			return partitionDevice, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return "", fmt.Errorf("partition device not found: %s", partitionDevice)
}
