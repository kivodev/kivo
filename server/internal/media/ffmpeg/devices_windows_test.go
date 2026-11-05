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

// Each Intel adapter is a Quick Sync device at its DXGI index, named by its
// index when there are two or more. Other vendors' adapters are left out.
func TestIntelDevices(t *testing.T) {
	const nvidia, basic = 0x10de, 0x1414
	for _, c := range []struct {
		list         []adapter
		nodes, names []string
	}{
		{nil, nil, nil},
		{[]adapter{{0, nvidia}, {1, basic}}, nil, nil},
		{[]adapter{{0, nvidia}, {1, intelVendor}, {2, basic}}, []string{"1"}, []string{"Intel Quick Sync (QSV)"}},
		{[]adapter{{0, intelVendor}, {1, intelVendor}}, []string{"0", "1"}, []string{"Intel Quick Sync (QSV) (0)", "Intel Quick Sync (QSV) (1)"}},
	} {
		var nodes, names []string
		for _, d := range intelDevices(c.list) {
			if d.b != qsvD3D11 {
				t.Errorf("%v: backend %s, want Quick Sync on D3D11", c.list, d.b.kind)
			}
			nodes, names = append(nodes, d.node), append(names, d.name)
		}
		if !slices.Equal(nodes, c.nodes) || !slices.Equal(names, c.names) {
			t.Errorf("%v: nodes %q and names %q, want %q and %q", c.list, nodes, names, c.nodes, c.names)
		}
	}
}

// DXGI lists the PC's adapters, each with a vendor.
func TestAdaptersListsGPUs(t *testing.T) {
	list := adapters()
	if len(list) == 0 {
		t.Fatal("DXGI listed no adapters")
	}
	for i, a := range list {
		if a.index != i || a.vendor == 0 {
			t.Errorf("adapter %d: %+v", i, a)
		}
	}
	t.Logf("adapters: %+v", list)
}
