package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	convertpkg "github.com/jelasin/vmdk-utils/internal/convert"
	"github.com/jelasin/vmdk-utils/internal/lvm"
	"github.com/jelasin/vmdk-utils/internal/mount"
	"github.com/jelasin/vmdk-utils/internal/nbd"
	"github.com/jelasin/vmdk-utils/internal/probe"
	"github.com/jelasin/vmdk-utils/internal/qemu"
	"github.com/jelasin/vmdk-utils/internal/state"
)

func RunRepack(out, errOut io.Writer, args []string) error {
	fs := flag.NewFlagSet("repack", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fromFormat := fs.String("from", "", "optional source image format")
	profile := fs.String("profile", "workstation", "output VMDK profile: workstation|esxi|stream-optimized")
	force := fs.Bool("force", false, "replace destination if it already exists")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 2 {
		return errors.New("usage: vmdkctl repack [--from <format>] [--profile workstation|esxi|stream-optimized] [--force] <src-image> <dst.vmdk>")
	}

	src, _ := filepath.Abs(fs.Arg(0))
	dst, _ := filepath.Abs(fs.Arg(1))
	if src == dst {
		return errors.New("source and destination must be different paths")
	}
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("stat source image: %w", err)
	}
	if _, err := os.Stat(dst); err == nil && !*force {
		return fmt.Errorf("destination already exists: %s (use --force to replace)", dst)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat destination image: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create destination dir: %w", err)
	}
	if err := ensureImageNotTracked(src, "source"); err != nil {
		return err
	}
	if err := ensureImageNotTracked(dst, "destination"); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".repack-*.vmdk")
	if err != nil {
		return fmt.Errorf("create temporary destination: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temporary destination: %w", err)
	}
	if err := os.Remove(tmpPath); err != nil {
		return fmt.Errorf("prepare temporary destination: %w", err)
	}
	keepTmp := false
	defer func() {
		if !keepTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := convertpkg.Convert(src, tmpPath, convertpkg.Options{
		FromFormat: *fromFormat,
		ToFormat:   "vmdk",
		Profile:    *profile,
	}); err != nil {
		return err
	}

	targets, err := validateRepackedVMDK(tmpPath)
	if err != nil {
		return err
	}

	if _, err := qemu.Inspect(tmpPath); err != nil {
		return fmt.Errorf("inspect repacked image: %w", err)
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		return fmt.Errorf("install repacked image: %w", err)
	}
	keepTmp = true

	_, err = fmt.Fprintf(out, "Repacked %s -> %s (profile=%s, validated mountable targets=%d)\n", src, dst, *profile, targets)
	return err
}

func ensureImageNotTracked(image, role string) error {
	store, err := state.Open()
	if err != nil {
		return err
	}
	session, ok := store.FindByImage(image)
	if !ok {
		return nil
	}
	health := session.Health()
	if health.Stale {
		return nil
	}
	return fmt.Errorf("%s image is currently tracked as %s on %s; unmount or detach it before repack", role, session.Status, session.Device)
}

func validateRepackedVMDK(image string) (int, error) {
	device, err := nbd.FindFreeDevice()
	if err != nil {
		return 0, err
	}
	if err := nbd.Attach(image, device, true); err != nil {
		return 0, fmt.Errorf("attach repacked image for validation: %w", err)
	}
	detach := true
	defer func() {
		if detach {
			_ = nbd.Detach(device)
		}
	}()

	targets, err := probe.MountTargets(device)
	if err != nil {
		return 0, fmt.Errorf("discover repacked mount targets: %w", err)
	}
	if len(targets) == 0 {
		return 0, errors.New("repacked image has no mountable filesystem targets")
	}

	vgSet := map[string]struct{}{}
	defer func() {
		_ = deactivateVGSet(vgSet)
	}()

	for _, target := range targets {
		for _, vg := range target.VGNames {
			vgSet[vg] = struct{}{}
		}
		tmpMount, err := os.MkdirTemp("", "vmdkctl-repack-check-")
		if err != nil {
			return 0, fmt.Errorf("create validation mountpoint: %w", err)
		}
		if err := mount.Mount(target.Device, tmpMount, true); err != nil {
			_ = os.Remove(tmpMount)
			return 0, fmt.Errorf("validate mount %s: %w", target.Device, err)
		}
		if err := mount.Umount(tmpMount); err != nil {
			_ = os.Remove(tmpMount)
			return 0, fmt.Errorf("validate unmount %s: %w", target.Device, err)
		}
		if err := os.Remove(tmpMount); err != nil {
			return 0, fmt.Errorf("remove validation mountpoint: %w", err)
		}
	}

	if err := deactivateVGSet(vgSet); err != nil {
		return 0, fmt.Errorf("deactivate validation volume groups: %w", err)
	}
	vgSet = nil

	if err := nbd.Detach(device); err != nil {
		return 0, fmt.Errorf("detach repacked validation device: %w", err)
	}
	detach = false
	return len(targets), nil
}

func deactivateVGSet(vgSet map[string]struct{}) error {
	if len(vgSet) == 0 {
		return nil
	}
	vgs := make([]string, 0, len(vgSet))
	for vg := range vgSet {
		vgs = append(vgs, vg)
	}
	return lvm.Deactivate(vgs)
}
