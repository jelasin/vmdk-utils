package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/jelasin/vmdk-utils/internal/nbd"
	"github.com/jelasin/vmdk-utils/internal/state"
)

func RunDetach(out, errOut io.Writer, args []string) error {
	fs := flag.NewFlagSet("detach", flag.ContinueOnError)
	fs.SetOutput(errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 1 {
		return errors.New("usage: vmdkctl detach <image|device>")
	}

	target := fs.Arg(0)
	store, err := state.Open()
	if err != nil {
		return err
	}

	device := target
	if !strings.HasPrefix(target, "/dev/") {
		session, ok := store.FindByImage(target)
		if !ok {
			return fmt.Errorf("no tracked session for image %s", target)
		}
		device = session.Device
	}

	if err := nbd.Detach(device); err != nil {
		return err
	}

	if err := store.RemoveByDevice(device); err != nil {
		return err
	}

	_, err = fmt.Fprintf(out, "Detached %s\n", device)
	return err
}
