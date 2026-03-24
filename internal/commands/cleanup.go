package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/jelasin/vmdk-utils/internal/state"
)

func RunCleanup(out, errOut io.Writer, args []string) error {
	fs := flag.NewFlagSet("cleanup", flag.ContinueOnError)
	fs.SetOutput(errOut)
	force := fs.Bool("force", false, "remove all tracked sessions without validation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: vmdkctl cleanup [--force]")
	}

	store, err := state.Open()
	if err != nil {
		return err
	}

	removed, kept, err := store.Cleanup(*force)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(out, "Cleanup finished: removed=%d kept=%d\n", removed, kept); err != nil {
		return err
	}
	return nil
}
