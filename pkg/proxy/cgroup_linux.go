//go:build linux

package proxy

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// linkoCgroupPath is the cgroupv2 directory used to identify the linko
// process for PID-aware loop prevention in nftables. The path is relative
// to the cgroupv2 mount point (/sys/fs/cgroup).
const linkoCgroupPath = "/sys/fs/cgroup/linko"

// setupLinkoCgroup creates a dedicated cgroupv2 directory for the linko
// process and moves the current process (and all its threads) into it.
// This allows nftables to match packets from linko's sockets using the
// socket cgroupv2 expression, enabling PID-aware loop prevention at the
// kernel level — the most reliable way to prevent the proxy's outbound
// connections from being DNATed back to itself.
//
// The cgroup inherits all resource limits from the parent (root cgroup),
// so this does NOT change any scheduling or resource constraints. It only
// sets the cgroup path that nftables uses to identify socket ownership.
//
// If cgroupv2 is unavailable or setup fails, the function logs a warning
// and returns nil — the caller should fall back to SO_MARK-based loop
// prevention in that case.
func setupLinkoCgroup() error {
	// Verify cgroupv2 is the active cgroup hierarchy.
	// cgroupv2 exposes /sys/fs/cgroup/cgroup.procs (not cgroup.procs inside
	// a subdirectory). Its absence indicates cgroupv1 or an un-mounted fs.
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.procs"); err != nil {
		slog.Warn("cgroupv2 not available, PID-based loop prevention disabled; "+
			"falling back to SO_MARK only",
			"error", err,
		)
		return nil
	}

	// On cgroupv2, mkdir creates a new child cgroup. The directory and its
	// contents are managed by cgroupfs — not a regular filesystem.
	if err := os.MkdirAll(linkoCgroupPath, 0755); err != nil {
		slog.Warn("cannot create linko cgroup, PID-based loop prevention disabled; "+
			"falling back to SO_MARK only",
			"path", linkoCgroupPath, "error", err,
		)
		return nil
	}

	// Move the current process into the linko cgroup. Writing a PID to
	// cgroup.procs moves the entire thread-group (all threads of the process)
	// into that cgroup. Every subsequently created socket will carry this
	// cgroup path in its kernel metadata.
	//
	// Must include a trailing newline; cgroupfs is strict about formatting.
	pid := os.Getpid()
	procsFile := filepath.Join(linkoCgroupPath, "cgroup.procs")
	if err := os.WriteFile(procsFile, []byte(fmt.Sprintf("%d\n", pid)), 0644); err != nil {
		slog.Warn("cannot move process to linko cgroup, PID-based loop "+
			"prevention disabled; falling back to SO_MARK only",
			"pid", pid, "error", err,
		)
		// Best-effort cleanup: remove the empty cgroup directory. On cgroupv2
		// an empty cgroup can be rmdir'd; we ignore the error if it fails.
		_ = os.Remove(linkoCgroupPath)
		return nil
	}

	slog.Info("PID-based loop prevention enabled via cgroupv2",
		"pid", pid,
		"cgroup", linkoCgroupPath,
	)
	return nil
}

// CleanupLinkoCgroup removes the linko cgroup directory. It is safe to call
// when linko is NOT running (e.g. from the cleanup command). When linko IS
// running, the cgroup still contains the live process and removal will fail
// with EBUSY — this is expected and harmless; the cgroup will persist until
// the process exits or is manually removed.
//
// Must be called only after nftables cleanup, because the running process's
// sockets still reference this cgroup path.
func CleanupLinkoCgroup() {
	if err := os.Remove(linkoCgroupPath); err != nil {
		if os.IsNotExist(err) {
			return // nothing to clean up
		}
		// EBUSY is expected when linko is still running — do not log as error.
		slog.Debug("removing linko cgroup (expected if process still running)",
			"path", linkoCgroupPath, "error", err,
		)
	}
}
