package scan

import "golang.org/x/sys/windows"

// PickerStarts returns where the library folder picker starts: the root of
// every drive, mapped network drives too.
func (s *Scanner) PickerStarts() []Directory {
	drives, _ := windows.GetLogicalDrives()
	var out []Directory
	for i := range 26 {
		if drives&(1<<i) != 0 {
			root := string(rune('A'+i)) + `:\`
			out = append(out, Directory{Name: root, Path: root})
		}
	}
	return out
}
