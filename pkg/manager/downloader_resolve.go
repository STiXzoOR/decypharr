package manager

import (
	"os"
	"path/filepath"

	"github.com/sirrobot01/decypharr/internal/utils"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

// resolveMountScanRoot picks the directory under the mount where the entry's
// files should be looked for. Decypharr's GetTorrentMountPath constructs the
// path from m.config.Mount.MountPath + "__all__" + entry.GetFolder(), where
// GetFolder() defaults to entry.Name. For single-file torrents whose entry.Name
// includes the file extension (e.g. "movie.mkv"), or when decypharr is pointed
// at a third-party mount (zurg, etc.) that names its folders from the torrent's
// display name rather than the canonical filename, the constructed mountPath
// does not exist on disk. The previous behaviour was to swallow the ENOENT
// inside checkDirectory and spin the poll loop until the 30-minute timeout —
// which on a FUSE-backed mount with thousands of entries in the parent stacks
// dentry locks and drives load average to four digits.
//
// This helper probes for an alternative directory derived from
// entry.OriginalFilename (the debrid provider's raw filename, which typically
// matches what zurg uses for its folder name) — both as-is and with the
// extension stripped. It returns the original mountPath unchanged when nothing
// usable is found, so the polling loop continues to wait for the expected path
// (the file may still be materializing).
func resolveMountScanRoot(entry *storage.Entry, mountPath string) string {
	if isDirectory(mountPath) {
		return mountPath
	}

	if entry == nil || entry.OriginalFilename == "" {
		return mountPath
	}

	parent := filepath.Dir(mountPath)
	seen := map[string]struct{}{mountPath: {}}

	candidates := []string{entry.OriginalFilename}
	if base := utils.RemoveExtension(entry.OriginalFilename); base != "" && base != entry.OriginalFilename {
		candidates = append(candidates, base)
	}

	for _, name := range candidates {
		candidate := filepath.Join(parent, name)
		if _, dup := seen[candidate]; dup {
			continue
		}
		seen[candidate] = struct{}{}
		if isDirectory(candidate) {
			return candidate
		}
	}

	return mountPath
}

// findMountFolderContainingFiles is the deep-scan fallback for resolveMountScanRoot.
// It walks the immediate children of parent and returns the first subdirectory
// whose own immediate children include a name present in targets.
//
// Cost: one readdir on parent plus one readdir per subdirectory. This is
// expensive on large mount roots (e.g. a /mnt/zurg/__all__/ with thousands of
// torrents) and should only be called as a last resort, gated on attempt
// count, when no cheaper candidate has resolved.
func findMountFolderContainingFiles(parent string, targets map[string]*storage.File) string {
	if len(targets) == 0 {
		return ""
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return ""
	}
	for _, item := range entries {
		if !item.IsDir() {
			continue
		}
		sub := filepath.Join(parent, item.Name())
		children, err := os.ReadDir(sub)
		if err != nil {
			continue
		}
		for _, child := range children {
			if _, hit := targets[child.Name()]; hit {
				return sub
			}
		}
	}
	return ""
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
