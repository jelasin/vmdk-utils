package commands

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jelasin/vmdk-utils/internal/lvm"
	"github.com/jelasin/vmdk-utils/internal/mount"
	"github.com/jelasin/vmdk-utils/internal/nbd"
	"github.com/jelasin/vmdk-utils/internal/probe"
	"github.com/jelasin/vmdk-utils/internal/state"
)

const mountAllManifestName = ".vmdkctl-mount-all.json"

type mountAllManifest struct {
	ImagePath string          `json:"imagePath"`
	Device    string          `json:"device"`
	Root      string          `json:"root"`
	ReadOnly  bool            `json:"readOnly"`
	Mounts    []mountAllEntry `json:"mounts"`
	VGNames   []string        `json:"vgNames,omitempty"`
}

type mountAllEntry struct {
	Name       string   `json:"name"`
	Device     string   `json:"device"`
	Source     string   `json:"source,omitempty"`
	Mountpoint string   `json:"mountpoint"`
	Partition  int      `json:"partition,omitempty"`
	VGNames    []string `json:"vgNames,omitempty"`
}

func RunMountAll(out, errOut io.Writer, args []string) error {
	fs := flag.NewFlagSet("mount-all", flag.ContinueOnError)
	fs.SetOutput(errOut)
	device := fs.String("device", "", "target /dev/nbdX device")
	readOnly := fs.Bool("read-only", true, "mount read-only")
	readWrite := fs.Bool("rw", false, "mount read-write")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 2 {
		return errors.New("usage: vmdkctl mount-all [--device /dev/nbdX] [--rw] <image> <dir>")
	}

	image := fs.Arg(0)
	root := fs.Arg(1)
	if *readWrite {
		*readOnly = false
	}
	if _, err := os.Stat(image); err != nil {
		return fmt.Errorf("stat image: %w", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create mount root: %w", err)
	}

	absoluteImage, _ := filepath.Abs(image)
	absoluteRoot, _ := filepath.Abs(root)
	store, err := state.Open()
	if err != nil {
		return err
	}
	if session, ok := store.FindByImage(absoluteImage); ok {
		return fmt.Errorf("image already tracked: %s -> %s", session.ImagePath, session.Device)
	}
	if session, ok := store.FindByMountpoint(absoluteRoot); ok {
		return fmt.Errorf("mountpoint already tracked: %s -> %s", absoluteRoot, session.ImagePath)
	}

	session, attachedNow, err := ensureSession(store, absoluteImage, *device, *readOnly)
	if err != nil {
		return err
	}

	targets, err := probe.MountTargets(session.Device)
	if err != nil || len(targets) == 0 {
		if attachedNow {
			_ = nbd.Detach(session.Device)
			_ = store.RemoveByDevice(session.Device)
		}
		if err != nil {
			return fmt.Errorf("no mountable partitions found: %w", err)
		}
		return fmt.Errorf("no mountable partitions found")
	}

	manifest := mountAllManifest{ImagePath: absoluteImage, Device: session.Device, Root: absoluteRoot, ReadOnly: *readOnly}
	nameCounts := map[string]int{}
	mounted := []mountAllEntry{}
	vgSet := map[string]struct{}{}
	for _, target := range targets {
		name := uniqueMountName(target, nameCounts)
		mountpoint := filepath.Join(absoluteRoot, name)
		if err := os.MkdirAll(mountpoint, 0o755); err != nil {
			_ = cleanupMountAll(manifest, store)
			return fmt.Errorf("create mountpoint %s: %w", mountpoint, err)
		}
		if err := mount.Mount(target.Device, mountpoint, *readOnly); err != nil {
			_ = cleanupMountAll(manifest, store)
			return fmt.Errorf("mount %s to %s: %w", target.Device, mountpoint, err)
		}
		entry := mountAllEntry{Name: name, Device: target.Device, Source: target.Source, Mountpoint: mountpoint, Partition: target.Partition, VGNames: append([]string(nil), target.VGNames...)}
		mounted = append(mounted, entry)
		manifest.Mounts = mounted
		for _, vg := range target.VGNames {
			vgSet[vg] = struct{}{}
		}
		manifest.VGNames = sortedKeys(vgSet)
	}
	manifest.VGNames = sortedKeys(vgSet)
	if err := writeMountAllManifest(absoluteRoot, manifest); err != nil {
		_ = cleanupMountAll(manifest, store)
		return err
	}

	session.Mountpoint = absoluteRoot
	session.LVMVGs = append([]string(nil), manifest.VGNames...)
	session.Status = "mounted-all"
	session.Partition = 0
	session.PartitionDevice = ""
	session.AutoDetected = false
	if err := store.Upsert(session); err != nil {
		_ = cleanupMountAll(manifest, store)
		return err
	}

	if _, err := fmt.Fprintf(out, "Mounted %d targets from %s under %s\n", len(manifest.Mounts), absoluteImage, absoluteRoot); err != nil {
		return err
	}
	for _, entry := range manifest.Mounts {
		if _, err := fmt.Fprintf(out, "- %s -> %s\n", entry.Device, entry.Mountpoint); err != nil {
			return err
		}
	}
	return nil
}

func cleanupMountAll(manifest mountAllManifest, store *state.Store) error {
	sort.Slice(manifest.Mounts, func(i, j int) bool { return manifest.Mounts[i].Mountpoint > manifest.Mounts[j].Mountpoint })
	for _, entry := range manifest.Mounts {
		_ = mount.Umount(entry.Mountpoint)
		_ = os.Remove(entry.Mountpoint)
	}
	if len(manifest.VGNames) > 0 {
		_ = lvm.Deactivate(manifest.VGNames)
	}
	if manifest.Device != "" {
		_ = nbd.Detach(manifest.Device)
		if store != nil {
			_ = store.RemoveByDevice(manifest.Device)
		}
	}
	if manifest.Root != "" {
		_ = os.Remove(manifestPath(manifest.Root))
	}
	return nil
}

func tryUmountMountAll(root string, store *state.Store) (bool, error) {
	manifest, err := readMountAllManifest(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return true, err
	}
	return true, cleanupMountAll(manifest, store)
}

func writeMountAllManifest(root string, manifest mountAllManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mount-all manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath(root), data, 0o644); err != nil {
		return fmt.Errorf("write mount-all manifest: %w", err)
	}
	return nil
}

func readMountAllManifest(root string) (mountAllManifest, error) {
	data, err := os.ReadFile(manifestPath(root))
	if err != nil {
		return mountAllManifest{}, err
	}
	var manifest mountAllManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return mountAllManifest{}, fmt.Errorf("parse mount-all manifest: %w", err)
	}
	return manifest, nil
}

func manifestPath(root string) string {
	return filepath.Join(root, mountAllManifestName)
}

func uniqueMountName(target probe.MountTarget, counts map[string]int) string {
	base := fmt.Sprintf("p%d", target.Partition)
	if target.Source != "" && target.Source != target.Device {
		base += "-" + sanitizeName(filepath.Base(target.Device))
	}
	counts[base]++
	if counts[base] == 1 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, counts[base])
}

func sanitizeName(value string) string {
	replacer := strings.NewReplacer("/", "-", " ", "-", "_", "-", ".", "-")
	value = replacer.Replace(value)
	value = strings.Trim(value, "-")
	if value == "" {
		return "fs"
	}
	return value
}

func sortedKeys(set map[string]struct{}) []string {
	result := make([]string, 0, len(set))
	for key := range set {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
