package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jelasin/vmdk-utils/internal/lvm"
	"github.com/jelasin/vmdk-utils/internal/nbd"
	"github.com/jelasin/vmdk-utils/internal/probe"
	"github.com/jelasin/vmdk-utils/internal/qemu"
	"github.com/jelasin/vmdk-utils/internal/runtime"
	"github.com/jelasin/vmdk-utils/internal/state"
)

func RunInspect(out, errOut io.Writer, args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(errOut)
	jsonOutput := fs.Bool("json", false, "emit machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 1 {
		return errors.New("usage: vmdkctl inspect [--json] <image>")
	}

	image := fs.Arg(0)
	if _, err := os.Stat(image); err != nil {
		return fmt.Errorf("stat image: %w", err)
	}
	absoluteImage, _ := filepath.Abs(image)

	info, err := qemu.Inspect(absoluteImage)
	if err != nil {
		return err
	}

	if *jsonOutput {
		_, err = fmt.Fprintln(out, info.JSON)
		return err
	}

	if _, err = fmt.Fprintln(out, info.Human); err != nil {
		return err
	}

	store, err := state.Open()
	if err != nil {
		return err
	}
	session, attachedNow, err := ensureSession(store, absoluteImage, "", true)
	if err != nil {
		return err
	}
	defer func() {
		if attachedNow {
			_ = nbd.Detach(session.Device)
			_ = store.RemoveByDevice(session.Device)
		}
	}()

	if _, err = fmt.Fprintf(out, "\nAttached device: %s\n", session.Device); err != nil {
		return err
	}
	inspectTargets := inspectBlockTargets(session.Device)
	if lsblkArgs := append([]string{"-o", "NAME,SIZE,TYPE,FSTYPE,UUID,MOUNTPOINT"}, inspectTargets...); len(inspectTargets) > 0 {
		if lsblk, err := runtime.RunCombined("lsblk", lsblkArgs...); err == nil && lsblk != "" {
			if _, err := fmt.Fprintf(out, "\nBlock devices:\n%s\n", lsblk); err != nil {
				return err
			}
		}
	}
	if blkid, err := runtime.RunCombined("blkid", inspectTargets...); err == nil && blkid != "" {
		if _, err := fmt.Fprintf(out, "\nblkid:\n%s\n", blkid); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, "\nPartition details:"); err != nil {
			return err
		}
		for _, line := range describePartitions(session.Device, blkid) {
			if _, err := fmt.Fprintln(out, line); err != nil {
				return err
			}
		}
	}
	if candidates, err := probe.Candidates(session.Device); err == nil && len(candidates) > 0 {
		if _, err := fmt.Fprintln(out, "\nAuto-detected candidates:"); err != nil {
			return err
		}
		for _, candidate := range candidates {
			if _, err := fmt.Fprintf(out, "- %s score=%d", candidate.Device, candidate.Score); err != nil {
				return err
			}
			if candidate.Partition > 0 {
				if _, err := fmt.Fprintf(out, " partition=%d", candidate.Partition); err != nil {
					return err
				}
			}
			if candidate.Source != "" && candidate.Source != candidate.Device {
				if _, err := fmt.Fprintf(out, " source=%s", candidate.Source); err != nil {
					return err
				}
			}
			if len(candidate.VGNames) > 0 {
				if _, err := fmt.Fprintf(out, " vg=%v", candidate.VGNames); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(out); err != nil {
				return err
			}
		}
	}
	if resolution, err := probe.Resolve(session.Device, 0); err == nil {
		if _, err := fmt.Fprintf(out, "\nSuggested root target: %s", resolution.Device); err != nil {
			return err
		}
		if resolution.Partition > 0 {
			if _, err := fmt.Fprintf(out, " (partition=%d)", resolution.Partition); err != nil {
				return err
			}
		}
		if len(resolution.VGNames) > 0 {
			if _, err := fmt.Fprintf(out, " vg=%v", resolution.VGNames); err != nil {
				return err
			}
			_ = lvm.Deactivate(resolution.VGNames)
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}
	return nil
}

func inspectBlockTargets(device string) []string {
	targets := []string{device}
	partitions, err := filepath.Glob(device + "p*")
	if err != nil || len(partitions) == 0 {
		return targets
	}

	sort.Slice(partitions, func(i, j int) bool {
		return inspectPartitionNumber(device, partitions[i]) < inspectPartitionNumber(device, partitions[j])
	})
	return append(targets, partitions...)
}

func inspectPartitionNumber(device, partitionDevice string) int {
	suffix := strings.TrimPrefix(partitionDevice, device+"p")
	n, err := strconv.Atoi(suffix)
	if err != nil {
		return 1<<31 - 1
	}
	return n
}

func describePartitions(device, blkidOutput string) []string {
	entries := parseBlkidOutput(blkidOutput)
	diskType := entries[device]["PTTYPE"]
	var lines []string
	for _, target := range inspectBlockTargets(device)[1:] {
		attrs, ok := entries[target]
		if !ok {
			lines = append(lines, fmt.Sprintf("- %s kind=%s", target, partitionKind(diskType, device, target, nil)))
			continue
		}
		parts := []string{fmt.Sprintf("- %s", target)}
		parts = append(parts, "kind="+partitionKind(diskType, device, target, attrs))
		if value := firstNonEmpty(attrs["TYPE"], attrs["PTTYPE"]); value != "" {
			parts = append(parts, "type="+value)
		}
		if value := attrs["LABEL"]; value != "" {
			parts = append(parts, "label="+value)
		}
		if value := attrs["UUID"]; value != "" {
			parts = append(parts, "uuid="+value)
		}
		if value := attrs["PARTUUID"]; value != "" {
			parts = append(parts, "partuuid="+value)
		}
		lines = append(lines, strings.Join(parts, " "))
	}
	return lines
}

func parseBlkidOutput(output string) map[string]map[string]string {
	result := map[string]map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		path := strings.TrimSpace(parts[0])
		attrs := map[string]string{}
		for _, field := range strings.Fields(parts[1]) {
			kv := strings.SplitN(field, "=", 2)
			if len(kv) != 2 {
				continue
			}
			attrs[kv[0]] = strings.Trim(kv[1], `"`)
		}
		result[path] = attrs
	}
	return result
}

func partitionKind(diskType, device, partitionDevice string, attrs map[string]string) string {
	if diskType == "dos" {
		n := inspectPartitionNumber(device, partitionDevice)
		if attrs != nil && attrs["PTTYPE"] == "dos" && attrs["TYPE"] == "" {
			return "extended"
		}
		if n >= 5 {
			return "logical"
		}
		if n > 0 {
			return "primary"
		}
	}
	return "partition"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
