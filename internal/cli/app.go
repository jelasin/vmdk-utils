package cli

import (
	"errors"
	"fmt"
	"io"
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
		a.printHelp()
		return nil
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "help", "-h", "--help":
		a.printHelp()
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
	case "repack":
		return commands.RunRepack(a.out, a.err, rest)
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

func (a *App) printHelp() {
	fmt.Fprintln(a.out, "vmdkctl - VMDK inspection and attachment tool")
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "Usage:")
	fmt.Fprintln(a.out, "  vmdkctl <command> [options]")
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "Commands:")
	fmt.Fprintln(a.out, "  inspect <image>          Show VMDK metadata via qemu-img")
	fmt.Fprintln(a.out, "  attach <image>           Attach image to a free /dev/nbdX")
	fmt.Fprintln(a.out, "  mount <image> <dir>      Attach and mount an image partition")
	fmt.Fprintln(a.out, "  mount-all <image> <dir>  Attach and mount all mountable partitions")
	fmt.Fprintln(a.out, "  umount <dir>             Unmount and detach tracked session")
	fmt.Fprintln(a.out, "  pull <image> ...         Copy files out of a guest partition")
	fmt.Fprintln(a.out, "  push <image> ...         Copy files into a guest partition")
	fmt.Fprintln(a.out, "  repack <src> <dst>       Convert/export image to VMDK")
	fmt.Fprintln(a.out, "  cleanup                  Remove stale tracked sessions")
	fmt.Fprintln(a.out, "  detach <image|device>    Detach an active /dev/nbdX session")
	fmt.Fprintln(a.out, "  status                   Show tracked sessions")
	fmt.Fprintln(a.out, "  detect-deps              Check required system dependencies")
	fmt.Fprintln(a.out)
}
