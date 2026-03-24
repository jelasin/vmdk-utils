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

func RunPull(out, errOut io.Writer, args []string) error {
	fs := flag.NewFlagSet("pull", flag.ContinueOnError)
	fs.SetOutput(errOut)
	partition := fs.Int("partition", 0, "partition number to mount (default: auto detect)")
	device := fs.String("device", "", "target /dev/nbdX device")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 3 {
		return errors.New("usage: vmdkctl pull [--device /dev/nbdX] [--partition N] <image> <guest-path> <local-path>")
	}

	image, _ := filepath.Abs(fs.Arg(0))
	guestPath := fs.Arg(1)
	localPath, _ := filepath.Abs(fs.Arg(2))

	store, err := state.Open()
	if err != nil {
		return err
	}
	tmpMount, err := os.MkdirTemp("", "vmdkctl-pull-")
	if err != nil {
		return fmt.Errorf("create temp mountpoint: %w", err)
	}
	defer os.Remove(tmpMount)

	session, attachedNow, err := ensureSession(store, image, *device, true)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = mount.Umount(tmpMount)
			if len(session.LVMVGs) > 0 {
				_ = lvm.Deactivate(session.LVMVGs)
			}
			if attachedNow {
				_ = nbd.Detach(session.Device)
				_ = store.RemoveByDevice(session.Device)
			}
		}
	}()

	resolution, err := probe.Resolve(session.Device, *partition)
	if err != nil {
		return err
	}
	session.Partition = resolution.Partition
	session.PartitionDevice = resolution.Device
	session.LVMVGs = resolution.VGNames
	session.AutoDetected = resolution.Auto
	if err := mount.Mount(resolution.Device, tmpMount, true); err != nil {
		return err
	}

	source := mount.ResolveGuestPath(tmpMount, guestPath)
	if err := mount.CopyOut(source, localPath); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(out, "Pulled %s:%s -> %s\n", image, guestPath, localPath); err != nil {
		return err
	}
	cleanup = true
	return nil
}
