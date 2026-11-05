package media

import "testing"

func TestLayout(t *testing.T) {
	for in, want := range map[string]string{
		"mono": "1.0", "stereo": "2.0", "downmix": "2.0", "quad": "4.0", "quad(side)": "4.0", "4.0": "4.0",
		"5.0(side)": "5.0", "5.1": "5.1", "5.1(side)": "5.1", "hexagonal": "6.0", "6.1(back)": "6.1",
		"7.1": "7.1", "7.1(wide)": "7.1", "7.1.4": "7.1.4", "octagonal": "8.0", "": "", "22.2": "22.2", "cube": "",
	} {
		if got := Layout(in); got != want {
			t.Errorf("Layout(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAudioProfile(t *testing.T) {
	for in, want := range map[string]string{
		"Dolby TrueHD + Dolby Atmos": "atmos", "Dolby Digital Plus + Dolby Atmos": "atmos",
		"DTS-HD MA + DTS:X": "dtsX", "DTS-HD MA + DTS:X IMAX": "dtsX", "DTS-HD MA": "dtsHdMa",
		"DTS-HD HRA": "dtsHdHra", "DTS-ES": "", "DTS": "", "LC": "", "": "",
	} {
		if got := AudioProfile(in); got != want {
			t.Errorf("AudioProfile(%q) = %q, want %q", in, got, want)
		}
	}
}
