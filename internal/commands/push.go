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

func RunPush(out, errOut io.Writer, args []string) error {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	fs.SetOutput(errOut)
	partition := fs.Int("partition", 0, "partition number to mount (default: auto detect)")
	device := fs.String("device", "", "target /dev/nbdX device")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 3 {
		return errors.New("usage: vmdkctl push [--device /dev/nbdX] [--partition N] <image> <local-path> <guest-path>")
	}

	image, _ := filepath.Abs(fs.Arg(0))
	localPath, _ := filepath.Abs(fs.Arg(1))
	guestPath := fs.Arg(2)
	if _, err := os.Stat(localPath); err != nil {
		return fmt.Errorf("stat local path: %w", err)
	}

	store, err := state.Open()
	if err != nil {
		return err
	}
	tmpMount, err := os.MkdirTemp("", "vmdkctl-push-")
	if err != nil {
		return fmt.Errorf("create temp mountpoint: %w", err)
	}
	defer os.Remove(tmpMount)

	session, attachedNow, err := ensureSession(store, image, *device, false)
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
	if err := mount.Mount(resolution.Device, tmpMount, false); err != nil {
		return err
	}

	target := mount.ResolveGuestPath(tmpMount, guestPath)
	if err := mount.CopyIn(localPath, target); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(out, "Pushed %s -> %s:%s\n", localPath, image, guestPath); err != nil {
		return err
	}
	cleanup = true
	return nil
}
