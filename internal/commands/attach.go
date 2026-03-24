package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jelasin/vmdk-utils/internal/nbd"
	"github.com/jelasin/vmdk-utils/internal/state"
)

func RunAttach(out, errOut io.Writer, args []string) error {
	fs := flag.NewFlagSet("attach", flag.ContinueOnError)
	fs.SetOutput(errOut)
	device := fs.String("device", "", "target /dev/nbdX device")
	readOnly := fs.Bool("read-only", true, "attach read-only")
	readWrite := fs.Bool("rw", false, "attach read-write")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 1 {
		return errors.New("usage: vmdkctl attach [--device /dev/nbdX] [--rw] <image>")
	}

	image := fs.Arg(0)
	if _, err := os.Stat(image); err != nil {
		return fmt.Errorf("stat image: %w", err)
	}
	if *readWrite {
		*readOnly = false
	}

	store, err := state.Open()
	if err != nil {
		return err
	}

	if session, ok := store.FindByImage(image); ok {
		return fmt.Errorf("image already attached: %s -> %s", session.ImagePath, session.Device)
	}

	selectedDevice := *device
	if selectedDevice == "" {
		selectedDevice, err = nbd.FindFreeDevice()
		if err != nil {
			return err
		}
	}

	if err := nbd.Attach(image, selectedDevice, *readOnly); err != nil {
		return err
	}

	session := state.Session{
		ImagePath: image,
		Device:    selectedDevice,
		ReadOnly:  *readOnly,
		Status:    "attached",
	}
	if err := store.Upsert(session); err != nil {
		_ = nbd.Detach(selectedDevice)
		return err
	}

	_, err = fmt.Fprintf(out, "Attached %s -> %s (readOnly=%t)\n", image, selectedDevice, *readOnly)
	return err
}
