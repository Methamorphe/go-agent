//go:build windows

package objectstore

func syncDirectory(string) error {
	// Object contents were already flushed before
	// the atomic rename.
	return nil
}
