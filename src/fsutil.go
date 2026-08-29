package main

import (
	"os"
	"path/filepath"
)

// atomicWriteFile writes data to a temp file in the same directory and renames it
// over path. A crash (OOM kill, power loss, SIGKILL) mid-write can therefore never
// leave a truncated or corrupt JSON file behind — the old file stays valid until
// the rename, and the rename itself is atomic on POSIX.
//
// This matters here because config.json and epg_mapping.json are rewritten
// regularly (EPG janitor, AI mapper, settings saves) and a single bad write used
// to destroy the whole mapping file (see CHANGELOG 1.5.x "recovered from backup").
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// No-op after a successful rename, but cleans up on every error path below.
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	// fsync before rename so the data hits disk before the file is swapped in.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
