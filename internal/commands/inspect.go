package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

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
	if lsblk, err := runtime.RunCombined("lsblk", "-o", "NAME,SIZE,TYPE,FSTYPE,UUID,MOUNTPOINT", session.Device); err == nil && lsblk != "" {
		if _, err := fmt.Fprintf(out, "\nBlock devices:\n%s\n", lsblk); err != nil {
			return err
		}
	}
	if blkid, err := runtime.RunCombined("blkid", session.Device, session.Device+"p1", session.Device+"p2", session.Device+"p3", session.Device+"p4"); err == nil && blkid != "" {
		if _, err := fmt.Fprintf(out, "\nblkid:\n%s\n", blkid); err != nil {
			return err
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
