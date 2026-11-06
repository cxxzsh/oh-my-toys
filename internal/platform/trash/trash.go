// Package trash moves files to the system's Trash (the Recycle Bin on
// Windows), from where people can put them back.
package trash

import (
	"fmt"

	"github.com/egoist/mygo"
)

// Move moves a file to the Trash.
func Move(path string) error {
	if err := mygo.Shell.TrashItem(path); err != nil {
		return fmt.Errorf("move %s to the trash: %w", path, err)
	}
	return nil
}
