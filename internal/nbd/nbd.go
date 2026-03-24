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
	matches, err := filepath.Glob("/sys/class/block/nbd*")
	if err != nil {
		return "", fmt.Errorf("glob nbd devices: %w", err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no /dev/nbdX devices found; load nbd module first")
	}

	for _, match := range matches {
		name := filepath.Base(match)
		pidPath := filepath.Join(match, "pid")
		if _, err := os.Stat(pidPath); os.IsNotExist(err) {
			return "/dev/" + name, nil
		}
		content, err := os.ReadFile(pidPath)
		if err == nil && strings.TrimSpace(string(content)) == "" {
			return "/dev/" + name, nil
		}
	}

	return "", fmt.Errorf("no free nbd devices available")
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
