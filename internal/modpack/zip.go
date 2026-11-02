package modpack

import (
	"archive/zip"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// archiveTime is a fixed timestamp so the same mod always zips to the same
// bytes; the 1980 epoch is the earliest a zip header can express.
var archiveTime = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// ZipPack writes dir as a .zip the engine's mod installer accepts: exactly one
// top-level folder (ModDirName) holding manifest.json at its root. Extra files
// outside that folder would make the installer reject the archive, so nothing
// else is added. The archive is written through a temporary file and renamed.
func Zip(dir, zipPath string) (files int, bytes int64, err error) {
	if dir == "" || zipPath == "" {
		return 0, 0, fmt.Errorf("missing mod directory or zip path")
	}
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		return 0, 0, fmt.Errorf("mod directory %s has no manifest.json: %w", dir, err)
	}
	if sameOrInside(dir, zipPath) {
		return 0, 0, fmt.Errorf("refusing to write %s inside the mod directory it archives (%s)", zipPath, dir)
	}
	if err := os.MkdirAll(filepath.Dir(zipPath), 0o755); err != nil {
		return 0, 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(zipPath), ".bwsprites-zip-*")
	if err != nil {
		return 0, 0, err
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		if err != nil {
			os.Remove(tmpPath)
		}
	}()
	zw := zip.NewWriter(tmp)
	walkErr := filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("mod file %s is not a regular file", p)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		hdr := &zip.FileHeader{
			Name:     ModDirName + "/" + filepath.ToSlash(rel),
			Method:   zip.Deflate,
			Modified: archiveTime,
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
		files++
		bytes += int64(len(data))
		return nil
	})
	if walkErr != nil {
		err = walkErr
		return 0, 0, err
	}
	if err = zw.Close(); err != nil {
		return 0, 0, err
	}
	if err = tmp.Close(); err != nil {
		return 0, 0, err
	}
	if err = os.Rename(tmpPath, zipPath); err != nil {
		return 0, 0, err
	}
	return files, bytes, nil
}
