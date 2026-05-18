package manager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirrobot01/decypharr/pkg/storage"
)

// TestResolveMountScanRoot covers the helper that locates the directory to
// scan when the path constructed via GetTorrentMountPath does not match the
// actual layout on the mount. The motivating case: decypharr is configured
// against a third-party mount like zurg, which uses the torrent's display
// name for its folder while decypharr's GetFolder() defaults to entry.Name
// (which for single-file torrents includes the .mkv extension and lower-case).
func TestResolveMountScanRoot(t *testing.T) {
	t.Run("mountPath exists, returns mountPath", func(t *testing.T) {
		tmpDir := t.TempDir()
		expected := filepath.Join(tmpDir, "real.folder")
		if err := os.MkdirAll(expected, 0o755); err != nil {
			t.Fatal(err)
		}

		entry := &storage.Entry{
			Name:             "real.folder",
			OriginalFilename: "Real.Folder.Display.Name",
		}

		if got := resolveMountScanRoot(entry, expected); got != expected {
			t.Errorf("resolveMountScanRoot = %q, want %q", got, expected)
		}
	})

	t.Run("zurg-style folder layout, OriginalFilename folder exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		actualFolder := filepath.Join(tmpDir, "Atlanta.S04E06.2160p.WEB.H265-GLHF")
		if err := os.MkdirAll(actualFolder, 0o755); err != nil {
			t.Fatal(err)
		}

		// What decypharr's GetTorrentMountPath computes — does not exist on this mount.
		expectedMountPath := filepath.Join(tmpDir, "atlanta.s04e06.2160p.web.h265-glhf.mkv")

		entry := &storage.Entry{
			Name:             "atlanta.s04e06.2160p.web.h265-glhf.mkv",
			OriginalFilename: "Atlanta.S04E06.2160p.WEB.H265-GLHF.mkv",
		}

		if got := resolveMountScanRoot(entry, expectedMountPath); got != actualFolder {
			t.Errorf("resolveMountScanRoot = %q, want %q", got, actualFolder)
		}
	})

	t.Run("OriginalFilename without extension matches existing folder", func(t *testing.T) {
		tmpDir := t.TempDir()
		actualFolder := filepath.Join(tmpDir, "Some.Movie.2024.BluRay")
		if err := os.MkdirAll(actualFolder, 0o755); err != nil {
			t.Fatal(err)
		}

		expectedMountPath := filepath.Join(tmpDir, "some.movie.2024.bluray.mkv")

		entry := &storage.Entry{
			Name:             "some.movie.2024.bluray.mkv",
			OriginalFilename: "Some.Movie.2024.BluRay.mkv",
		}

		if got := resolveMountScanRoot(entry, expectedMountPath); got != actualFolder {
			t.Errorf("resolveMountScanRoot = %q, want %q", got, actualFolder)
		}
	})

	t.Run("neither mountPath nor any candidate exists, returns mountPath", func(t *testing.T) {
		tmpDir := t.TempDir()
		expectedMountPath := filepath.Join(tmpDir, "atlanta.s04e06.2160p.web.h265-glhf.mkv")

		entry := &storage.Entry{
			Name:             "atlanta.s04e06.2160p.web.h265-glhf.mkv",
			OriginalFilename: "Atlanta.S04E06.2160p.WEB.H265-GLHF.mkv",
		}

		if got := resolveMountScanRoot(entry, expectedMountPath); got != expectedMountPath {
			t.Errorf("resolveMountScanRoot = %q, want %q (mountPath unchanged)", got, expectedMountPath)
		}
	})

	t.Run("empty OriginalFilename, returns mountPath unchanged", func(t *testing.T) {
		tmpDir := t.TempDir()
		expectedMountPath := filepath.Join(tmpDir, "anything.mkv")

		entry := &storage.Entry{
			Name:             "anything.mkv",
			OriginalFilename: "",
		}

		if got := resolveMountScanRoot(entry, expectedMountPath); got != expectedMountPath {
			t.Errorf("resolveMountScanRoot = %q, want %q (no alternatives to try)", got, expectedMountPath)
		}
	})

	t.Run("OriginalFilename equals entry.Name basename, does not duplicate or loop", func(t *testing.T) {
		tmpDir := t.TempDir()
		expectedMountPath := filepath.Join(tmpDir, "same.name")

		entry := &storage.Entry{
			Name:             "same.name",
			OriginalFilename: "same.name",
		}

		if got := resolveMountScanRoot(entry, expectedMountPath); got != expectedMountPath {
			t.Errorf("resolveMountScanRoot = %q, want %q", got, expectedMountPath)
		}
	})

	t.Run("OriginalFilename matches but is a file, not a directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Edge: a sibling file with the candidate name. We should not treat a
		// regular file as a valid scan root.
		siblingFile := filepath.Join(tmpDir, "Sibling.Name")
		if err := os.WriteFile(siblingFile, []byte("not a directory"), 0o644); err != nil {
			t.Fatal(err)
		}

		expectedMountPath := filepath.Join(tmpDir, "sibling.name.mkv")

		entry := &storage.Entry{
			Name:             "sibling.name.mkv",
			OriginalFilename: "Sibling.Name",
		}

		if got := resolveMountScanRoot(entry, expectedMountPath); got != expectedMountPath {
			t.Errorf("resolveMountScanRoot = %q, want %q (candidate is a file, not a dir)", got, expectedMountPath)
		}
	})
}

// TestFindMountFolderContainingFiles covers the deep-scan fallback: when the
// expected mount path and the OriginalFilename-derived candidates have all
// failed, walk the parent directory and return the first folder whose
// immediate children include any of the target file names.
func TestFindMountFolderContainingFiles(t *testing.T) {
	t.Run("finds folder containing a target file at first level", func(t *testing.T) {
		tmpDir := t.TempDir()

		decoy := filepath.Join(tmpDir, "decoy.folder")
		if err := os.MkdirAll(decoy, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(decoy, "unrelated.mkv"), []byte{}, 0o644); err != nil {
			t.Fatal(err)
		}

		target := filepath.Join(tmpDir, "actual.folder")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "wanted.mkv"), []byte{}, 0o644); err != nil {
			t.Fatal(err)
		}

		targets := map[string]*storage.File{
			"wanted.mkv": {Name: "wanted.mkv"},
		}

		if got := findMountFolderContainingFiles(tmpDir, targets); got != target {
			t.Errorf("findMountFolderContainingFiles = %q, want %q", got, target)
		}
	})

	t.Run("returns empty string when no folder matches", func(t *testing.T) {
		tmpDir := t.TempDir()
		decoy := filepath.Join(tmpDir, "decoy.folder")
		if err := os.MkdirAll(decoy, 0o755); err != nil {
			t.Fatal(err)
		}

		targets := map[string]*storage.File{
			"wanted.mkv": {Name: "wanted.mkv"},
		}

		if got := findMountFolderContainingFiles(tmpDir, targets); got != "" {
			t.Errorf("findMountFolderContainingFiles = %q, want empty", got)
		}
	})

	t.Run("returns empty when parent does not exist", func(t *testing.T) {
		got := findMountFolderContainingFiles(filepath.Join(os.TempDir(), "definitely-not-here-XXXXX"), nil)
		if got != "" {
			t.Errorf("findMountFolderContainingFiles = %q, want empty", got)
		}
	})

	t.Run("ignores non-directory siblings in parent", func(t *testing.T) {
		tmpDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(tmpDir, "wanted.mkv"), []byte{}, 0o644); err != nil {
			t.Fatal(err)
		}

		targets := map[string]*storage.File{
			"wanted.mkv": {Name: "wanted.mkv"},
		}

		if got := findMountFolderContainingFiles(tmpDir, targets); got != "" {
			t.Errorf("findMountFolderContainingFiles = %q, want empty (file at parent, not in a subfolder)", got)
		}
	})

	t.Run("empty target map returns empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		decoy := filepath.Join(tmpDir, "decoy.folder")
		if err := os.MkdirAll(decoy, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(decoy, "anything.mkv"), []byte{}, 0o644); err != nil {
			t.Fatal(err)
		}

		if got := findMountFolderContainingFiles(tmpDir, map[string]*storage.File{}); got != "" {
			t.Errorf("findMountFolderContainingFiles = %q, want empty (no targets requested)", got)
		}
	})
}
