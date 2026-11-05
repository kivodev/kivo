//go:build !windows

package scan

import (
	"bufio"
	"os"
	"slices"
	"strings"
)

// pseudoFS are filesystem types that never hold media.
var pseudoFS = []string{
	"proc", "sysfs", "devtmpfs", "devpts", "tmpfs", "cgroup", "cgroup2", "mqueue", "securityfs",
	"debugfs", "tracefs", "pstore", "bpf", "configfs", "fusectl", "hugetlbfs", "autofs", "binfmt_misc",
	"overlay", "nsfs", "rpc_pipefs", "efivarfs", "ramfs", "squashfs",
}

// PickerStarts returns where the library folder picker starts: / plus every
// mount that could hold media. That way /media and /mnt/… show at once in a
// container.
func (s *Scanner) PickerStarts() []Directory {
	out := []Directory{{Name: "/", Path: "/"}}
	f, err := os.Open("/proc/self/mounts")
	if err != nil {
		return out // not on Linux
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 || slices.Contains(pseudoFS, fields[2]) {
			continue
		}
		mount := strings.ReplaceAll(fields[1], `\040`, " ")
		if mount == "/" || strings.HasPrefix(mount, "/proc") || strings.HasPrefix(mount, "/sys") ||
			strings.HasPrefix(mount, "/dev") || strings.HasPrefix(mount, "/etc") || strings.HasPrefix(mount, "/boot") {
			continue
		}
		if !slices.ContainsFunc(out, func(d Directory) bool { return d.Path == mount }) {
			out = append(out, Directory{Name: mount, Path: mount})
		}
	}
	slices.SortFunc(out[1:], func(a, b Directory) int { return strings.Compare(a.Path, b.Path) })
	return out
}
