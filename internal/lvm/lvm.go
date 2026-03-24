package lvm

import (
	"fmt"
	"strings"

	"github.com/jelasin/vmdk-utils/internal/runtime"
)

func IsPhysicalVolume(device string) bool {
	output, err := runtime.RunCombined("blkid", device)
	if err != nil {
		return false
	}
	return strings.Contains(output, `TYPE="LVM2_member"`)
}

func ActivateForPV(device string) ([]string, error) {
	vgNames, err := VolumeGroupsForPV(device)
	if err != nil {
		return nil, err
	}
	if len(vgNames) == 0 {
		return nil, fmt.Errorf("no volume group found for %s", device)
	}
	for _, vg := range vgNames {
		if _, err := runtime.RunCombined("vgchange", "-ay", vg); err != nil {
			return nil, fmt.Errorf("activate volume group %s: %w", vg, err)
		}
	}
	return vgNames, nil
}

func VolumeGroupsForPV(device string) ([]string, error) {
	output, err := runtime.RunCombined("pvs", "--noheadings", "-o", "vg_name", device)
	if err != nil {
		return nil, fmt.Errorf("query pvs for %s: %w", device, err)
	}
	return nonEmptyLines(output), nil
}

func LogicalVolumes(vgName string) ([]string, error) {
	output, err := runtime.RunCombined("lvs", "--noheadings", "-o", "lv_path", vgName)
	if err != nil {
		return nil, fmt.Errorf("query lvs for %s: %w", vgName, err)
	}
	return nonEmptyLines(output), nil
}

func LogicalVolumesForMount(vgNames []string) ([]string, error) {
	var result []string
	seen := map[string]struct{}{}
	for _, vgName := range vgNames {
		lvs, err := LogicalVolumes(vgName)
		if err != nil {
			return nil, err
		}
		for _, lv := range lvs {
			if _, ok := seen[lv]; ok {
				continue
			}
			seen[lv] = struct{}{}
			result = append(result, lv)
		}
	}
	return result, nil
}

func Deactivate(vgNames []string) error {
	for _, vg := range vgNames {
		if _, err := runtime.RunCombined("vgchange", "-an", vg); err != nil {
			return fmt.Errorf("deactivate volume group %s: %w", vg, err)
		}
	}
	return nil
}

func nonEmptyLines(output string) []string {
	lines := strings.Split(output, "\n")
	result := make([]string, 0, len(lines))
	seen := map[string]struct{}{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		result = append(result, line)
	}
	return result
}
