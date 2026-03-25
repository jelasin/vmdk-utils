package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	convertpkg "github.com/jelasin/vmdk-utils/internal/convert"
)

func RunConvert(out, errOut io.Writer, args []string) error {
	fs := flag.NewFlagSet("convert", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fromFormat := fs.String("from", "", "optional source image format")
	toFormat := fs.String("to", "", "destination image format")
	profile := fs.String("profile", "workstation", "export profile: workstation|esxi|stream-optimized")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 2 {
		return errors.New("usage: vmdkctl convert --to <format> [--from <format>] [--profile workstation|esxi|stream-optimized] <src-image> <dst-image>")
	}
	if *toFormat == "" {
		return errors.New("--to is required")
	}

	src, _ := filepath.Abs(fs.Arg(0))
	dst, _ := filepath.Abs(fs.Arg(1))
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("stat source image: %w", err)
	}
	if *profile != "workstation" && *toFormat != "vmdk" {
		return errors.New("--profile is only supported when --to vmdk")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create destination dir: %w", err)
	}

	if err := convertpkg.Convert(src, dst, convertpkg.Options{
		FromFormat: *fromFormat,
		ToFormat:   *toFormat,
		Profile:    *profile,
	}); err != nil {
		return err
	}

	message := fmt.Sprintf("Converted %s -> %s (to=%s", src, dst, *toFormat)
	if *toFormat == "vmdk" {
		message += fmt.Sprintf(", profile=%s", *profile)
	}
	message += ")\n"
	_, err := io.WriteString(out, message)
	return err
}
