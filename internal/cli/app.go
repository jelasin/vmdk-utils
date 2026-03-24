package cli

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jelasin/vmdk-utils/internal/commands"
)

type App struct {
	out io.Writer
	err io.Writer
}

func NewApp(out, err io.Writer) *App {
	return &App{out: out, err: err}
}

func (a *App) Run(args []string) error {
	if len(args) == 0 {
		a.printHelp(nil)
		return nil
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "help", "-h", "--help":
		a.printHelp(rest)
		return nil
	case "inspect":
		return commands.RunInspect(a.out, a.err, rest)
	case "attach":
		return commands.RunAttach(a.out, a.err, rest)
	case "mount":
		return commands.RunMount(a.out, a.err, rest)
	case "mount-all":
		return commands.RunMountAll(a.out, a.err, rest)
	case "umount":
		return commands.RunUmount(a.out, a.err, rest)
	case "pull":
		return commands.RunPull(a.out, a.err, rest)
	case "push":
		return commands.RunPush(a.out, a.err, rest)
	case "convert":
		return commands.RunConvert(a.out, a.err, rest)
	case "cleanup":
		return commands.RunCleanup(a.out, a.err, rest)
	case "detach":
		return commands.RunDetach(a.out, a.err, rest)
	case "status":
		return commands.RunStatus(a.out, a.err, rest)
	case "detect-deps":
		return commands.RunDetectDeps(a.out, a.err, rest)
	default:
		if strings.HasPrefix(cmd, "-") {
			return errors.New("unknown global option: " + cmd)
		}
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func (a *App) printHelp(args []string) {
	if len(args) > 0 {
		if text, ok := commandHelp(args[0]); ok {
			fmt.Fprint(a.out, text)
			return
		}
	}

	fmt.Fprintln(a.out, "vmdkctl - VMDK inspection and attachment tool")
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "Usage:")
	fmt.Fprintln(a.out, "  vmdkctl <command> [options]")
	fmt.Fprintln(a.out, "  vmdkctl help <command>")
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "Commands:")
	for _, name := range commandOrder() {
		summary := commandSummaries()[name]
		fmt.Fprintf(a.out, "  %-23s %s\n", name, summary)
	}
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "Run `vmdkctl help <command>` for detailed flags and examples.")
}

func commandOrder() []string {
	return []string{"inspect", "attach", "mount", "mount-all", "umount", "pull", "push", "convert", "cleanup", "detach", "status", "detect-deps"}
}

func commandSummaries() map[string]string {
	return map[string]string{
		"inspect":     "Show VMDK metadata and detected partitions",
		"attach":      "Attach image to a /dev/nbdX device",
		"mount":       "Attach image and mount one filesystem",
		"mount-all":   "Attach image and mount every mountable filesystem",
		"umount":      "Unmount tracked mountpoint and detach device",
		"pull":        "Copy files out of a guest filesystem",
		"push":        "Copy files into a guest filesystem",
		"convert":     "Convert/export an image to VMDK",
		"cleanup":     "Remove stale tracked sessions",
		"detach":      "Disconnect an active /dev/nbdX session",
		"status":      "Show tracked sessions and health",
		"detect-deps": "Check required system tools and kernel support",
	}
}

func commandHelp(name string) (string, bool) {
	help := map[string]string{
		"inspect": `Usage:
  vmdkctl inspect [--json] <image>

Options:
  --json    Print qemu-img JSON output instead of human-readable text

Notes:
  - Temporarily attaches the image to inspect partitions
  - Shows block devices, blkid output, partition details, and auto-detected candidates

`,
		"attach": `Usage:
  vmdkctl attach [--device /dev/nbdX] [--read-only] [--rw] <image>

Options:
  --device      Attach to a specific /dev/nbdX instead of auto-selecting one
  --read-only   Attach read-only (default)
  --rw          Attach read-write

`,
		"mount": `Usage:
  vmdkctl mount [--device /dev/nbdX] [--partition N] [--read-only] [--rw] <image> <mountpoint>

Options:
  --device      Reuse or force a specific /dev/nbdX
  --partition   Mount a specific partition number; default is auto-detect
  --read-only   Mount read-only (default)
  --rw          Mount read-write and persist changes into the source image

`,
		"mount-all": `Usage:
  vmdkctl mount-all [--device /dev/nbdX] [--read-only] [--rw] <image> <mount-root>

Options:
  --device      Reuse or force a specific /dev/nbdX
  --read-only   Mount read-only (default)
  --rw          Mount read-write

Notes:
  - Creates subdirectories like p1, p5, p6 under the given root
  - Use 'vmdkctl umount <mount-root>' to unmount the whole set

`,
		"umount": `Usage:
  vmdkctl umount <mountpoint>

Notes:
  - For a single mount, unmounts the filesystem and detaches the backing NBD device
  - For mount-all roots, unmounts all generated submounts and removes their subdirectories

`,
		"pull": `Usage:
  vmdkctl pull [--device /dev/nbdX] [--partition N] <image> <guest-path> <local-path>

Notes:
  - Attaches and mounts the image temporarily, then copies a file or directory out

`,
		"push": `Usage:
  vmdkctl push [--device /dev/nbdX] [--partition N] <image> <local-path> <guest-path>

Notes:
  - Attaches and mounts the image read-write temporarily, then copies content in

`,
		"convert": `Usage:
  vmdkctl convert [--profile workstation|esxi|stream-optimized] [--input-format fmt] <src-image> <dst.vmdk>

Options:
  --profile       Output VMDK profile; default is workstation
  --input-format  Optional source format passed to qemu-img

Notes:
  - Converts or exports an image into a new VMDK file
  - Does not modify the source image in place

`,
		"cleanup": `Usage:
  vmdkctl cleanup [--force]

Options:
  --force    Remove all tracked sessions without validating them

Notes:
  - Without --force, only stale sessions are removed from the state file

`,
		"detach": `Usage:
  vmdkctl detach <image|device>

Notes:
  - Disconnects qemu-nbd for the target /dev/nbdX
  - Removes the matching tracked session

`,
		"status": `Usage:
  vmdkctl status

Notes:
  - Shows tracked sessions, mountpoints, LVM metadata, and health status

`,
		"detect-deps": `Usage:
  vmdkctl detect-deps

Notes:
  - Checks required and optional external tools in PATH
  - Verifies kernel NBD support and suggests install commands when tools are missing

`,
	}
	if name == "commands" {
		keys := make([]string, 0, len(help))
		for key := range help {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return strings.Join(keys, "\n") + "\n", true
	}
	text, ok := help[name]
	return text, ok
}
