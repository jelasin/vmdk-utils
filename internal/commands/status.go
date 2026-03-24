package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"

	"github.com/jelasin/vmdk-utils/internal/state"
)

func RunStatus(out, errOut io.Writer, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: vmdkctl status")
	}

	store, err := state.Open()
	if err != nil {
		return err
	}

	sessions := store.Sessions()
	if len(sessions) == 0 {
		_, err := fmt.Fprintln(out, "No tracked sessions.")
		return err
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Device < sessions[j].Device
	})

	for _, session := range sessions {
		health := session.Health()
		if _, err := fmt.Fprintf(out, "%s\t%s\treadOnly=%t\tstatus=%s\tpartition=%d\tauto=%t\tmount=%s\tvgs=%v\thealth=%s\n", session.Device, session.ImagePath, session.ReadOnly, session.Status, session.Partition, session.AutoDetected, session.Mountpoint, session.LVMVGs, health.String()); err != nil {
			return err
		}
	}

	return nil
}
