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
		pidPath := filepath.Join(match, "pid")
		if _, err := os.Stat(pidPath); os.IsNotExist(err) {
			// Check if device has a valid size (not disconnected)
			sizePath := filepath.Join(match, "size")
			sizeBytes, err := os.ReadFile(sizePath)
			if err == nil {
				size := strings.TrimSpace(string(sizeBytes))
				if size != "0" {
					// Device is in a bad state (has size but no pid), skip it
					continue
				}
			}
			return "/dev/" + name, nil
		}
		content, err := os.ReadFile(pidPath)
		if err == nil && strings.TrimSpace(string(content)) == "" {
			// Check if device has a valid size (not disconnected)
			sizePath := filepath.Join(match, "size")
			sizeBytes, err := os.ReadFile(sizePath)
			if err == nil {
				size := strings.TrimSpace(string(sizeBytes))
				if size != "0" {
					// Device is in a bad state (has size but empty pid), skip it
					continue
				}
			}
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
	return nil
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
