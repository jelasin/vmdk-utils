package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jelasin/vmdk-utils/internal/lvm"
	"github.com/jelasin/vmdk-utils/internal/mount"
	"github.com/jelasin/vmdk-utils/internal/nbd"
	"github.com/jelasin/vmdk-utils/internal/probe"
	"github.com/jelasin/vmdk-utils/internal/state"
)

func RunMount(out, errOut io.Writer, args []string) error {
	fs := flag.NewFlagSet("mount", flag.ContinueOnError)
	fs.SetOutput(errOut)
	device := fs.String("device", "", "target /dev/nbdX device")
	partition := fs.Int("partition", 0, "partition number to mount (default: auto detect)")
	readOnly := fs.Bool("read-only", true, "mount read-only")
	readWrite := fs.Bool("rw", false, "mount read-write")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 2 {
		return errors.New("usage: vmdkctl mount [--device /dev/nbdX] [--partition N] [--rw] <image> <mountpoint>")
	}

	image := fs.Arg(0)
	mountpoint := fs.Arg(1)
	if *readWrite {
		*readOnly = false
	}

	if _, err := os.Stat(image); err != nil {
		return fmt.Errorf("stat image: %w", err)
	}
	if err := os.MkdirAll(mountpoint, 0o755); err != nil {
		return fmt.Errorf("create mountpoint: %w", err)
	}
	absoluteImage, _ := filepath.Abs(image)
	absoluteMount, _ := filepath.Abs(mountpoint)

	store, err := state.Open()
	if err != nil {
		return err
	}
	if session, ok := store.FindByMountpoint(absoluteMount); ok {
		return fmt.Errorf("mountpoint already tracked: %s -> %s", absoluteMount, session.ImagePath)
	}

	session, attachedNow, err := ensureSession(store, absoluteImage, *device, *readOnly)
	if err != nil {
		return err
	}

	resolution, err := probe.Resolve(session.Device, *partition)
	if err != nil {
		if attachedNow {
			if len(session.LVMVGs) > 0 {
				_ = lvm.Deactivate(session.LVMVGs)
			}
			_ = nbd.Detach(session.Device)
			_ = store.RemoveByDevice(session.Device)
		}
		return err
	}

	if err := mount.Mount(resolution.Device, absoluteMount, *readOnly); err != nil {
		if len(resolution.VGNames) > 0 {
			_ = lvm.Deactivate(resolution.VGNames)
		}
		if attachedNow {
			_ = nbd.Detach(session.Device)
			_ = store.RemoveByDevice(session.Device)
		}
		return err
	}

	session.Partition = resolution.Partition
	session.PartitionDevice = resolution.Device
	session.Mountpoint = absoluteMount
	session.LVMVGs = resolution.VGNames
	session.AutoDetected = resolution.Auto
	session.Status = "mounted"
	if err := store.Upsert(session); err != nil {
		_ = mount.Umount(absoluteMount)
		if len(resolution.VGNames) > 0 {
			_ = lvm.Deactivate(resolution.VGNames)
		}
		if attachedNow {
			_ = nbd.Detach(session.Device)
		}
		return err
	}

	_, err = fmt.Fprintf(out, "Mounted %s (%s) -> %s\n", absoluteImage, resolution.Device, absoluteMount)
	return err
}
