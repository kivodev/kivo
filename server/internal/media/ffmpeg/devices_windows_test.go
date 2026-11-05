package ffmpeg

import (
	"slices"
	"testing"
)

// Each index nvidia-smi lists is an NVENC device, named by its index when
// there are two or more.
func TestNVIDIADevices(t *testing.T) {
	for _, c := range []struct {
		out          string
		nodes, names []string
	}{
		{"", nil, nil},
		{"No devices were found\r\n", nil, nil},
		{"0\r\n", []string{"0"}, []string{"NVIDIA NVENC"}},
		{"0\r\n1\r\n", []string{"0", "1"}, []string{"NVIDIA NVENC (0)", "NVIDIA NVENC (1)"}},
	} {
		var nodes, names []string
		for _, d := range nvidiaDevices(c.out) {
			if d.b != nvenc {
				t.Errorf("%q: backend %s, want NVENC", c.out, d.b.kind)
			}
			nodes, names = append(nodes, d.node), append(names, d.name)
		}
		if !slices.Equal(nodes, c.nodes) || !slices.Equal(names, c.names) {
			t.Errorf("%q: nodes %q and names %q, want %q and %q", c.out, nodes, names, c.nodes, c.names)
		}
	}
}
