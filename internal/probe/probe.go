package probe

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jelasin/vmdk-utils/internal/lvm"
	"github.com/jelasin/vmdk-utils/internal/mount"
	"github.com/jelasin/vmdk-utils/internal/nbd"
	"github.com/jelasin/vmdk-utils/internal/runtime"
)

type Resolution struct {
	Device    string
	Partition int
	VGNames   []string
	Source    string
	Auto      bool
}

type Candidate struct {
	Device    string
	Partition int
	VGNames   []string
	Source    string
	Score     int
}

type MountTarget struct {
	Device    string
	Partition int
	VGNames   []string
	Source    string
}

type blockDevices struct {
	Blockdevices []blockDevice `json:"blockdevices"`
}

type blockDevice struct {
	Path     string        `json:"path"`
	Name     string        `json:"name"`
	Type     string        `json:"type"`
	FSType   string        `json:"fstype"`
	Children []blockDevice `json:"children"`
}

type candidate struct {
	device    string
	partition int
	vgNames   []string
	source    string
	score     int
}

func Resolve(device string, partition int) (Resolution, error) {
	if partition > 0 {
		return resolveExplicit(device, partition)
	}
	return resolveAutomatic(device)
}

func Candidates(device string) ([]Candidate, error) {
	raw, err := waitForCandidates(device)
	if err != nil {
		return nil, err
	}
	result := make([]Candidate, 0, len(raw))
	for _, item := range raw {
		result = append(result, Candidate{
			Device:    item.device,
			Partition: item.partition,
			VGNames:   item.vgNames,
			Source:    item.source,
			Score:     item.score,
		})
	}
	return result, nil
}

func MountTargets(device string) ([]MountTarget, error) {
	partitions, err := partitions(device)
	if err != nil {
		return nil, err
	}
	result := make([]MountTarget, 0, len(partitions))
	seen := map[string]struct{}{}
	for _, part := range partitions {
		partNum := extractPartitionNumber(device, part.Path)
		if partNum == 0 {
			continue
		}
		if lvm.IsPhysicalVolume(part.Path) {
			vgNames, err := lvm.ActivateForPV(part.Path)
			if err != nil {
				continue
			}
			lvs, err := lvm.LogicalVolumesForMount(vgNames)
			if err != nil {
				_ = lvm.Deactivate(vgNames)
				continue
			}
			for _, lv := range lvs {
				if _, ok := seen[lv]; ok || !isMountableFilesystem(lv) {
					continue
				}
				seen[lv] = struct{}{}
				result = append(result, MountTarget{Device: lv, Partition: partNum, VGNames: vgNames, Source: part.Path})
			}
			continue
		}
		if _, ok := seen[part.Path]; ok || !isMountableFilesystem(part.Path) {
			continue
		}
		seen[part.Path] = struct{}{}
		result = append(result, MountTarget{Device: part.Path, Partition: partNum, Source: part.Path})
	}
	return result, nil
}

func resolveExplicit(device string, partition int) (Resolution, error) {
	partitionDevice, err := nbd.WaitForPartition(device, partition)
	if err != nil {
		return Resolution{}, err
	}
	if lvm.IsPhysicalVolume(partitionDevice) {
		vgNames, err := lvm.ActivateForPV(partitionDevice)
		if err != nil {
			return Resolution{}, err
		}
		best, err := bestLogicalVolume(vgNames)
		if err != nil {
			_ = lvm.Deactivate(vgNames)
			return Resolution{}, err
		}
		return Resolution{Device: best.device, Partition: partition, VGNames: vgNames, Source: partitionDevice, Auto: false}, nil
	}
	return Resolution{Device: partitionDevice, Partition: partition, Source: partitionDevice, Auto: false}, nil
}

func resolveAutomatic(device string) (Resolution, error) {
	candidates, err := waitForCandidates(device)
	if err != nil {
		return Resolution{}, err
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	best := candidates[0]
	return Resolution{Device: best.device, Partition: best.partition, VGNames: best.vgNames, Source: best.source, Auto: true}, nil
}

func waitForCandidates(device string) ([]candidate, error) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		candidates, err := candidateList(device)
		if err != nil {
			return nil, err
		}
		if len(candidates) > 0 {
			return candidates, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("no suitable filesystem candidate found; specify --partition manually")
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func candidateList(device string) ([]candidate, error) {
	partitions, err := partitions(device)
	if err != nil {
		return nil, err
	}
	candidates := []candidate{}
	for _, part := range partitions {
		partNum := extractPartitionNumber(device, part.Path)
		if partNum == 0 {
			continue
		}
		if lvm.IsPhysicalVolume(part.Path) {
			vgNames, err := lvm.ActivateForPV(part.Path)
			if err != nil {
				continue
			}
			lvs, err := logicalVolumeCandidates(vgNames, partNum, part.Path)
			if err != nil {
				_ = lvm.Deactivate(vgNames)
				continue
			}
			candidates = append(candidates, lvs...)
			continue
		}
		score := scoreFilesystem(part.Path)
		if score > 0 {
			candidates = append(candidates, candidate{device: part.Path, partition: partNum, source: part.Path, score: score})
		}
	}
	return candidates, nil
}

func bestLogicalVolume(vgNames []string) (candidate, error) {
	candidates, err := logicalVolumeCandidates(vgNames, 0, "")
	if err != nil {
		return candidate{}, err
	}
	if len(candidates) == 0 {
		return candidate{}, fmt.Errorf("no logical volume candidate found")
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	return candidates[0], nil
}

func logicalVolumeCandidates(vgNames []string, partition int, source string) ([]candidate, error) {
	var result []candidate
	lvs, err := lvm.LogicalVolumesForMount(vgNames)
	if err != nil {
		return nil, err
	}
	for _, lv := range lvs {
		score := scoreFilesystem(lv)
		if score == 0 {
			continue
		}
		result = append(result, candidate{device: lv, partition: partition, vgNames: vgNames, source: source, score: score})
	}
	return result, nil
}

func isMountableFilesystem(device string) bool {
	tmpMount, err := os.MkdirTemp("", "vmdkctl-check-")
	if err != nil {
		return false
	}
	defer os.Remove(tmpMount)
	if err := mount.Mount(device, tmpMount, true); err != nil {
		return false
	}
	defer func() { _ = mount.Umount(tmpMount) }()
	return true
}

func scoreFilesystem(device string) int {
	tmpMount, err := os.MkdirTemp("", "vmdkctl-probe-")
	if err != nil {
		return 0
	}
	defer os.Remove(tmpMount)
	if err := mount.Mount(device, tmpMount, true); err != nil {
		return 0
	}
	defer func() { _ = mount.Umount(tmpMount) }()

	score := 0
	if mount.Exists(filepath.Join(tmpMount, "etc", "os-release")) {
		score += 5
	}
	if mount.Exists(filepath.Join(tmpMount, "etc", "fstab")) {
		score += 4
	}
	if mount.Exists(filepath.Join(tmpMount, "usr")) {
		score += 2
	}
	if mount.Exists(filepath.Join(tmpMount, "var")) {
		score += 2
	}
	if mount.Exists(filepath.Join(tmpMount, "bin")) || mount.Exists(filepath.Join(tmpMount, "sbin")) {
		score += 1
	}
	if mount.Exists(filepath.Join(tmpMount, "boot")) {
		score += 1
	}
	return score
}

func partitions(device string) ([]blockDevice, error) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		partitions, err := listPartitions(device)
		if err == nil && len(partitions) > 0 {
			return partitions, nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("no partitions found for %s", device)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func listPartitions(device string) ([]blockDevice, error) {
	result := []blockDevice{}
	seen := map[string]struct{}{}

	output, err := runtime.RunCombined("lsblk", "-J", "-p", "-o", "PATH,NAME,TYPE,FSTYPE", device)
	if err != nil {
		return nil, fmt.Errorf("lsblk %s: %w", device, err)
	}
	var parsed blockDevices
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		return nil, fmt.Errorf("parse lsblk json: %w", err)
	}
	if len(parsed.Blockdevices) == 0 {
		return nil, fmt.Errorf("no block device data for %s", device)
	}

	for _, part := range flattenPartitions(parsed.Blockdevices[0]) {
		if part.Path == "" {
			continue
		}
		seen[part.Path] = struct{}{}
		result = append(result, part)
	}

	globbed, err := filepath.Glob(device + "p*")
	if err != nil {
		return nil, fmt.Errorf("glob partitions for %s: %w", device, err)
	}
	for _, part := range globbed {
		if _, ok := seen[part]; ok {
			continue
		}
		result = append(result, blockDevice{Path: part, Name: filepath.Base(part), Type: "part"})
	}

	sort.Slice(result, func(i, j int) bool {
		return extractPartitionNumber(device, result[i].Path) < extractPartitionNumber(device, result[j].Path)
	})
	return result, nil
}

func flattenPartitions(root blockDevice) []blockDevice {
	result := []blockDevice{}
	for _, child := range root.Children {
		if child.Type == "part" {
			result = append(result, child)
		}
		result = append(result, flattenPartitions(child)...)
	}
	return result
}

func extractPartitionNumber(device, partitionDevice string) int {
	suffix := strings.TrimPrefix(partitionDevice, device+"p")
	if suffix == partitionDevice {
		return 0
	}
	var n int
	_, _ = fmt.Sscanf(suffix, "%d", &n)
	return n
}
