package identify

import "io/fs"

// SetOpenFolder replaces how i opens a library folder to read NFO files.
func (i *Identifier) SetOpenFolder(open func(path string) (fs.FS, func() error, error)) {
	i.openFolder = open
}
