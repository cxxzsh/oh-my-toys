package core

import (
	"os"
	"path/filepath"
)

// PortableMarker names the file that makes the app portable: next to the
// executable, it keeps the app's settings and data in a folder beside it,
// instead of the user's application data. The portable zip ships it; the
// installer does not.
const PortableMarker = "portable"

// portableData names the folder, next to the executable, where a portable
// app keeps its data.
const portableData = "data"

// PortableDir returns where the app at exe keeps its data when it is
// portable, and whether it is: the data folder next to it, when a regular
// file named PortableMarker is there too.
func PortableDir(exe string) (string, bool) {
	dir := filepath.Dir(exe)
	info, err := os.Stat(filepath.Join(dir, PortableMarker))
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return filepath.Join(dir, portableData), true
}
