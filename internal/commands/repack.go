package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jelasin/vmdk-utils/internal/repack"
)

func RunRepack(out, errOut io.Writer, args []string) error {
	fs := flag.NewFlagSet("repack", flag.ContinueOnError)
	fs.SetOutput(errOut)
	profile := fs.String("profile", "workstation", "export profile: workstation|esxi|stream-optimized")
	inputFormat := fs.String("input-format", "", "optional source image format")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 2 {
		return errors.New("usage: vmdkctl repack [--profile workstation|esxi|stream-optimized] [--input-format fmt] <src-image> <dst.vmdk>")
	}

	src, _ := filepath.Abs(fs.Arg(0))
	dst, _ := filepath.Abs(fs.Arg(1))
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("stat source image: %w", err)
	}
	if filepath.Ext(dst) != ".vmdk" {
		return errors.New("destination must use .vmdk extension")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create destination dir: %w", err)
	}

	if err := repack.ConvertToVMDK(src, dst, repack.Options{
		Profile:     *profile,
		InputFormat: *inputFormat,
	}); err != nil {
		return err
	}

	_, err := fmt.Fprintf(out, "Repacked %s -> %s (profile=%s)\n", src, dst, *profile)
	return err
}
