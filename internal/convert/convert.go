package convert

import (
	"fmt"

	"github.com/jelasin/vmdk-utils/internal/runtime"
)

type Options struct {
	FromFormat string
	ToFormat   string
	Profile    string
}

func Convert(src, dst string, opts Options) error {
	args := []string{"convert", "-p"}
	if opts.FromFormat != "" {
		args = append(args, "-f", opts.FromFormat)
	}
	if opts.ToFormat == "" {
		return fmt.Errorf("destination format is required")
	}
	args = append(args, "-O", opts.ToFormat)

	profileOptions, err := profileArgs(opts.ToFormat, opts.Profile)
	if err != nil {
		return err
	}
	if profileOptions != "" {
		args = append(args, "-o", profileOptions)
	}
	args = append(args, src, dst)

	if _, err := runtime.RunCombined("qemu-img", args...); err != nil {
		return fmt.Errorf("qemu-img convert to %s: %w", opts.ToFormat, err)
	}
	return nil
}

func profileArgs(toFormat, profile string) (string, error) {
	if toFormat != "vmdk" {
		if profile == "" || profile == "workstation" {
			return "", nil
		}
		return "", fmt.Errorf("unsupported convert profile %q for output format %q", profile, toFormat)
	}

	switch profile {
	case "", "workstation":
		return "adapter_type=lsilogic,subformat=monolithicSparse", nil
	case "esxi":
		return "adapter_type=lsilogic,subformat=streamOptimized,compat6", nil
	case "stream-optimized":
		return "subformat=streamOptimized", nil
	default:
		return "", fmt.Errorf("unsupported convert profile %q", profile)
	}
}
