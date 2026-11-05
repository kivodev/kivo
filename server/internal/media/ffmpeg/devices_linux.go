package ffmpeg

import (
	"errors"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/mmcdole/kivo/internal/media"
)

// PCI vendor IDs, as sysfs writes them.
const (
	vendorIntel  = "0x8086"
	vendorAMD    = "0x1002"
	vendorNVIDIA = "0x10de"
)

// findDevices lists the GPUs under r.root that might encode. It also lists
// the GPUs that can't be used even before any test:
//   - a render node missing from /dev, since sysfs shows the host's GPUs
//     even in a container;
//   - a render node the server may not open;
//   - an NVIDIA GPU without its driver's device nodes.
//
// An Intel render node is tried as Quick Sync and as VA-API. A usable
// device is named for the method it's used by. A GPU that can't be used is
// named for itself.
func (r *Runner) findDevices() ([]foundDevice, []media.AcceleratorProblem) {
	var found []foundDevice
	var problems []media.AcceleratorProblem
	sysNodes, _ := filepath.Glob(filepath.Join(r.root, "sys/class/drm/renderD*"))
	for _, sys := range sysNodes {
		base := filepath.Base(sys)
		vendor := readLine(filepath.Join(sys, "device/vendor"))
		var bs []*backend
		name := "GPU (" + base + ")"
		switch vendor {
		case vendorNVIDIA:
			continue // NVENC is reached through CUDA, handled below
		case vendorIntel:
			bs, name = []*backend{qsv, vaapi}, "Intel "+name
		case vendorAMD:
			bs, name = []*backend{vaapi}, "AMD "+name
		default:
			bs = []*backend{vaapi}
		}
		node := "/dev/dri/" + base
		problem := media.AcceleratorProblem{Accelerator: acceleratorOf(bs[0], node), Backend: bs[0].kind, Name: name}
		if p, ok := r.openProblem(node, problem); ok {
			problems = append(problems, p)
			continue
		}
		for _, b := range bs {
			found = append(found, foundDevice{b: b, node: node, name: methodName(vendor, b.kind)})
		}
	}

	gpus, _ := filepath.Glob(filepath.Join(r.root, "dev/nvidia[0-9]*"))
	slices.SortFunc(gpus, func(a, b string) int { return nvidiaIndex(a) - nvidiaIndex(b) })
	names := nvidiaNames(r.root)
	for i, path := range gpus {
		idx := strconv.Itoa(nvidiaIndex(path))
		name := "NVIDIA GPU " + idx
		if i < len(names) {
			name = names[i]
		}
		problem := media.AcceleratorProblem{Accelerator: acceleratorOf(nvenc, idx), Backend: media.NVENC, Name: name}
		if _, err := os.Stat(filepath.Join(r.root, "dev/nvidiactl")); err != nil {
			problem.Reason, problem.Detail = media.NoDriver, "/dev/nvidiactl is missing: in Docker, run the container with the NVIDIA runtime"
			problems = append(problems, problem)
			continue
		}
		if p, ok := r.openProblem("/dev/nvidia"+idx, problem); ok {
			problems = append(problems, p)
			continue
		}
		found = append(found, foundDevice{b: nvenc, node: idx, name: "NVIDIA NVENC"})
	}
	if len(gpus) == 0 && r.hasNVIDIA() {
		problems = append(problems, media.AcceleratorProblem{Accelerator: acceleratorOf(nvenc, "0"), Backend: media.NVENC, Name: "NVIDIA GPU",
			Reason: media.NoDriver, Detail: "an NVIDIA GPU is installed but /dev/nvidia0 is missing: in Docker, run the container with the NVIDIA runtime"})
	}
	// Two GPUs of one kind would share a name, so tell them apart by node.
	seen := map[string]int{}
	for _, c := range found {
		seen[c.name]++
	}
	for i, c := range found {
		if seen[c.name] > 1 {
			found[i].name += " (" + filepath.Base(c.node) + ")"
		}
	}
	return found, problems
}

// methodName names a render node's device by its vendor and backend, such
// as "Intel Quick Sync (QSV)", "Intel VA-API" or "AMD VA-API".
func methodName(vendor string, b media.Backend) string {
	switch {
	case b == media.QSV:
		return "Intel Quick Sync (QSV)"
	case vendor == vendorIntel:
		return "Intel VA-API"
	case vendor == vendorAMD:
		return "AMD VA-API"
	}
	return "VA-API"
}

// openProblem checks that the server can open node. If it can't, it fills
// in the reason.
func (r *Runner) openProblem(node string, p media.AcceleratorProblem) (media.AcceleratorProblem, bool) {
	path := filepath.Join(r.root, node)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	switch {
	case err == nil:
		f.Close()
		return p, false
	case errors.Is(err, fs.ErrNotExist):
		p.Reason, p.Detail = media.NoDeviceNode, node+" is missing"
	case errors.Is(err, fs.ErrPermission):
		p.Reason, p.Detail, p.Group = media.NoPermission, "can't open "+node+": permission denied", group(path)
		if p.Group != "" {
			p.Detail += ", owned by group " + p.Group
		}
	default:
		p.Reason, p.Detail = media.NoDriver, err.Error()
	}
	return p, true
}

// group names the group that owns path, with its ID. A container's
// --group-add needs the ID.
func group(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return ""
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	gid := strconv.FormatUint(uint64(st.Gid), 10)
	if g, err := user.LookupGroupId(gid); err == nil {
		return g.Name + " (gid " + gid + ")"
	}
	return "gid " + gid
}

// hasNVIDIA reports whether an NVIDIA display controller is on the PCI bus.
func (r *Runner) hasNVIDIA() bool {
	devs, _ := filepath.Glob(filepath.Join(r.root, "sys/bus/pci/devices/*"))
	for _, d := range devs {
		if readLine(filepath.Join(d, "vendor")) == vendorNVIDIA && strings.HasPrefix(readLine(filepath.Join(d, "class")), "0x03") {
			return true
		}
	}
	return false
}

// nvidiaNames lists the driver's names for its GPUs, in bus order, which is
// the order of their device nodes.
func nvidiaNames(root string) []string {
	infos, _ := filepath.Glob(filepath.Join(root, "proc/driver/nvidia/gpus/*/information"))
	slices.Sort(infos)
	var names []string
	for _, info := range infos {
		b, _ := os.ReadFile(info)
		for line := range strings.Lines(string(b)) {
			if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "Model" {
				names = append(names, strings.TrimSpace(v))
			}
		}
	}
	return names
}

func nvidiaIndex(path string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(filepath.Base(path), "nvidia"))
	return n
}

func readLine(path string) string {
	b, _ := os.ReadFile(path)
	return strings.TrimSpace(string(b))
}
