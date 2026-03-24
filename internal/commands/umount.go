package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/jelasin/vmdk-utils/internal/lvm"
	"github.com/jelasin/vmdk-utils/internal/mount"
	"github.com/jelasin/vmdk-utils/internal/nbd"
	"github.com/jelasin/vmdk-utils/internal/state"
)

func RunUmount(out, errOut io.Writer, args []string) error {
	fs := flag.NewFlagSet("umount", flag.ContinueOnError)
	fs.SetOutput(errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 1 {
		return errors.New("usage: vmdkctl umount <mountpoint>")
	}

	mountpoint, _ := filepath.Abs(fs.Arg(0))
	store, err := state.Open()
	if err != nil {
		return err
	}
	session, ok := store.FindByMountpoint(mountpoint)
	if !ok {
		return fmt.Errorf("no tracked session for mountpoint %s", mountpoint)
	}

	if err := mount.Umount(mountpoint); err != nil {
		return err
	}
	if len(session.LVMVGs) > 0 {
		if err := lvm.Deactivate(session.LVMVGs); err != nil {
			return err
		}
	}
	if err := nbd.Detach(session.Device); err != nil {
		return err
	}
	if err := store.RemoveByDevice(session.Device); err != nil {
		return err
	}

	_, err = fmt.Fprintf(out, "Unmounted %s and detached %s\n", mountpoint, session.Device)
	return err
}
