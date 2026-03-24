package repack

import (
	"fmt"

	"github.com/jelasin/vmdk-utils/internal/runtime"
)

type Options struct {
	Profile     string
	InputFormat string
}

func ConvertToVMDK(src, dst string, opts Options) error {
	profileOptions, err := profileArgs(opts.Profile)
	if err != nil {
		return err
	}

	args := []string{"convert", "-p"}
	if opts.InputFormat != "" {
		args = append(args, "-f", opts.InputFormat)
	}
	args = append(args, "-O", "vmdk")
	if profileOptions != "" {
		args = append(args, "-o", profileOptions)
	}
	args = append(args, src, dst)

	if _, err := runtime.RunCombined("qemu-img", args...); err != nil {
		return fmt.Errorf("qemu-img convert to vmdk: %w", err)
	}
	return nil
}

func profileArgs(profile string) (string, error) {
	switch profile {
	case "", "workstation":
		return "adapter_type=lsilogic,subformat=monolithicSparse", nil
	case "esxi":
		return "adapter_type=lsilogic,subformat=streamOptimized,compat6", nil
	case "stream-optimized":
		return "subformat=streamOptimized", nil
	default:
		return "", fmt.Errorf("unsupported repack profile %q", profile)
	}
}
