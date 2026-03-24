package commands

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
)

type depCheck struct {
	name        string
	required    bool
	description string
}

type distroInfo struct {
	id     string
	idLike []string
	pretty string
}

func RunDetectDeps(out, errOut io.Writer, args []string) error {
	fs := flag.NewFlagSet("detect-deps", flag.ContinueOnError)
	fs.SetOutput(errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: vmdkctl detect-deps")
	}

	checks := []depCheck{
		{name: "qemu-img", required: true, description: "inspect and convert VMDK images"},
		{name: "qemu-nbd", required: true, description: "attach and detach images as NBD devices"},
		{name: "partprobe", required: true, description: "trigger partition rescans after attach"},
		{name: "lsblk", required: true, description: "enumerate disk partitions"},
		{name: "mount", required: true, description: "mount guest filesystems"},
		{name: "umount", required: true, description: "unmount guest filesystems"},
		{name: "modprobe", required: true, description: "load the nbd kernel module when needed"},
		{name: "blkid", required: false, description: "detect filesystem and LVM physical volumes"},
		{name: "pvs", required: false, description: "discover LVM volume groups"},
		{name: "lvs", required: false, description: "list LVM logical volumes"},
		{name: "vgchange", required: false, description: "activate and deactivate LVM groups"},
	}

	missingRequired := false
	missingOptional := false
	missingTools := []string{}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "NAME\tKIND\tSTATUS\tLOCATION\tDETAIL"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(tw, "----\t----\t------\t--------\t------"); err != nil {
		return err
	}
	for _, check := range checks {
		path, err := exec.LookPath(check.name)
		status := "ok"
		location := path
		if err != nil {
			location = "missing"
			missingTools = append(missingTools, check.name)
			if check.required {
				status = "missing-required"
				missingRequired = true
			} else {
				status = "missing-optional"
				missingOptional = true
			}
		}
		kind := "optional"
		if check.required {
			kind = "required"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", check.name, kind, status, location, check.description); err != nil {
			return err
		}
	}

	moduleStatus, moduleDetail := detectNBDModule()
	if _, err := fmt.Fprintf(tw, "nbd-module\trequired\t%s\t%s\t%s\n", moduleStatus, moduleDetail, "kernel NBD support"); err != nil {
		return err
	}
	if moduleStatus != "ok" {
		missingRequired = true
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(missingTools) > 0 || moduleStatus != "ok" {
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		for _, line := range installHints(missingTools, moduleStatus) {
			if _, err := fmt.Fprintln(out, line); err != nil {
				return err
			}
		}
	}

	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if missingRequired {
		_, err := fmt.Fprintln(out, "Dependency check: missing required dependencies")
		return err
	}
	if missingOptional {
		_, err := fmt.Fprintln(out, "Dependency check: required dependencies are present; some optional LVM tools are missing")
		return err
	}
	_, err := fmt.Fprintln(out, "Dependency check: all required and optional dependencies are present")
	return err
}

func installHints(missingTools []string, moduleStatus string) []string {
	distro := detectDistro()
	packages := packageListForDistro(distro, missingTools)
	lines := []string{}
	if distro.pretty != "" {
		lines = append(lines, "Detected distro: "+distro.pretty)
	}
	if len(packages) > 0 {
		lines = append(lines, "Suggested install command:")
		lines = append(lines, "  "+installCommandForDistro(distro, packages))
	}
	if moduleStatus != "ok" {
		lines = append(lines, "NBD module hint:")
		lines = append(lines, "  sudo modprobe nbd max_part=16")
	}
	if len(lines) == 0 {
		lines = append(lines, "Missing dependencies detected, but no distro-specific install hint is available.")
	}
	return lines
}

func detectDistro() distroInfo {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return distroInfo{}
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		values[parts[0]] = strings.Trim(parts[1], `"`)
	}
	info := distroInfo{
		id:     values["ID"],
		pretty: values["PRETTY_NAME"],
	}
	if like := values["ID_LIKE"]; like != "" {
		info.idLike = strings.Fields(like)
	}
	return info
}

func packageListForDistro(distro distroInfo, tools []string) []string {
	mapping := packageMappingForDistro(distro)
	seen := map[string]struct{}{}
	packages := []string{}
	for _, tool := range tools {
		pkg, ok := mapping[tool]
		if !ok || pkg == "" {
			continue
		}
		if _, exists := seen[pkg]; exists {
			continue
		}
		seen[pkg] = struct{}{}
		packages = append(packages, pkg)
	}
	return packages
}

func packageMappingForDistro(distro distroInfo) map[string]string {
	family := distroFamily(distro)
	switch family {
	case "debian":
		return map[string]string{
			"qemu-img":  "qemu-utils",
			"qemu-nbd":  "qemu-utils",
			"partprobe": "parted",
			"lsblk":     "util-linux",
			"mount":     "util-linux",
			"umount":    "util-linux",
			"modprobe":  "kmod",
			"blkid":     "util-linux",
			"pvs":       "lvm2",
			"lvs":       "lvm2",
			"vgchange":  "lvm2",
		}
	case "rhel":
		return map[string]string{
			"qemu-img":  "qemu-img",
			"qemu-nbd":  "qemu-nbd",
			"partprobe": "parted",
			"lsblk":     "util-linux",
			"mount":     "util-linux",
			"umount":    "util-linux",
			"modprobe":  "kmod",
			"blkid":     "util-linux",
			"pvs":       "lvm2",
			"lvs":       "lvm2",
			"vgchange":  "lvm2",
		}
	case "arch":
		return map[string]string{
			"qemu-img":  "qemu",
			"qemu-nbd":  "qemu",
			"partprobe": "parted",
			"lsblk":     "util-linux",
			"mount":     "util-linux",
			"umount":    "util-linux",
			"modprobe":  "kmod",
			"blkid":     "util-linux",
			"pvs":       "lvm2",
			"lvs":       "lvm2",
			"vgchange":  "lvm2",
		}
	default:
		return map[string]string{}
	}
}

func installCommandForDistro(distro distroInfo, packages []string) string {
	family := distroFamily(distro)
	args := strings.Join(packages, " ")
	switch family {
	case "debian":
		return "sudo apt update && sudo apt install -y " + args
	case "rhel":
		return "sudo dnf install -y " + args
	case "arch":
		return "sudo pacman -S --needed " + args
	default:
		return "install packages: " + args
	}
}

func distroFamily(distro distroInfo) string {
	candidates := append([]string{distro.id}, distro.idLike...)
	for _, candidate := range candidates {
		switch candidate {
		case "ubuntu", "debian":
			return "debian"
		case "rhel", "fedora", "centos", "rocky", "almalinux":
			return "rhel"
		case "arch":
			return "arch"
		}
	}
	return ""
}

func detectNBDModule() (string, string) {
	if _, err := os.Stat("/sys/module/nbd"); err == nil {
		return "ok", "loaded"
	}
	modprobePath, err := exec.LookPath("modprobe")
	if err != nil {
		return "missing-required", "modprobe missing"
	}
	cmd := exec.Command(modprobePath, "-n", "nbd")
	output, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			text = err.Error()
		}
		return "missing-required", text
	}
	return "ok", "available (not loaded)"
}
