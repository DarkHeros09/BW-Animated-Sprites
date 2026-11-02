package modpack

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The engine's installer accepts a .zip only when it holds exactly one
// top-level folder with manifest.json at its root; every other shape is
// rejected before it is installed.
func TestZipWritesSingleFolderArchive(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "pack")
	writeTestPack(t, pack)
	dir := filepath.Join(root, "mod")
	if _, err := Assemble(pack, dir, false); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(root, "dist", "bwsprites.zip")
	files, bytes, err := Zip(dir, zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if files != 6 || bytes <= 0 {
		t.Fatalf("zip wrote %d files, %d bytes; want 6 files", files, bytes)
	}
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	found := map[string]bool{}
	tops := map[string]bool{}
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			t.Errorf("archive holds a directory entry %q", f.Name)
		}
		if strings.Contains(f.Name, "\\") {
			t.Errorf("archive entry %q uses a backslash", f.Name)
		}
		parts := strings.Split(f.Name, "/")
		if len(parts) < 2 {
			t.Errorf("archive entry %q sits outside a mod folder", f.Name)
			continue
		}
		tops[parts[0]] = true
		found[f.Name] = true
	}
	if len(tops) != 1 || !tops[ModDirName] {
		t.Fatalf("top-level entries %v, want only %q", tops, ModDirName)
	}
	for _, rel := range []string{"manifest.json", "main.lua", "pack.lua", "index.lua", "front/1/normal.png"} {
		if !found[ModDirName+"/"+rel] {
			t.Errorf("archive has no %s/%s", ModDirName, rel)
		}
	}
	got, err := r.Open(ModDirName + "/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	buf := make([]byte, 1<<12)
	n, _ := got.Read(buf)
	want, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != string(want) {
		t.Error("the archived manifest differs from the assembled one")
	}
	again := filepath.Join(root, "dist", "again.zip")
	if _, _, err := Zip(dir, again); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(again)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Error("zipping the same mod twice produced different archives")
	}
	names, err := os.ReadDir(filepath.Dir(zipPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if strings.HasPrefix(name.Name(), ".bwsprites-zip-") {
			t.Errorf("a temporary file %q was left behind", name.Name())
		}
	}
}

func TestZipExternalModHasFourFilesAndOneFolder(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "mod")
	if _, err := AssembleExternal(dir, false); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(root, "dist", "bwsprites-bw.zip")
	files, _, err := Zip(dir, zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if files != 4 {
		t.Fatalf("zip wrote %d files, want 4", files)
	}
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	found := map[string]bool{}
	for _, f := range r.File {
		found[f.Name] = true
	}
	for _, rel := range []string{"manifest.json", "main.lua", "pack.lua", "external.lua"} {
		if !found[ModDirName+"/"+rel] {
			t.Errorf("archive has no %s/%s", ModDirName, rel)
		}
	}
	if found[ModDirName+"/index.lua"] {
		t.Error("the code-only archive embeds an index.lua")
	}
}

func TestZipRejectsBadInputs(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "mod")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Zip(dir, filepath.Join(root, "out.zip")); err == nil ||
		!strings.Contains(err.Error(), "no manifest.json") {
		t.Fatalf("a mod directory without a manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Zip(dir, filepath.Join(dir, "out.zip")); err == nil ||
		!strings.Contains(err.Error(), "inside the mod directory") {
		t.Fatalf("a zip inside the mod directory: %v", err)
	}
	if _, _, err := Zip("", "out.zip"); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("an empty directory argument: %v", err)
	}
}
