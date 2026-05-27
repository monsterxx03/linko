package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove linko firewall rules and disable pf",
	Long: `Cleanup removes all linko pf firewall rules and disables the packet filter.

This command is useful for recovering from unexpected crashes or SIGKILL signals
where linko did not get a chance to clean up its firewall rules gracefully.

If linko was killed with SIGKILL (kill -9), or the process crashed, the pf
redirect rules may still be active, causing network traffic to be redirected
to a proxy that is no longer running. Running 'linko cleanup' will restore
normal network connectivity by flushing the linko pf anchor and disabling pf.

Requires root privileges (sudo).`,
	Run: runCleanup,
}

func runCleanup(cmd *cobra.Command, args []string) {
	const anchorName = "com.apple/linko"
	const confPath = "/etc/pf.linko.conf"
	const timeout = 10 * time.Second

	var hasError bool

	// Step 1: Flush the linko pf anchor
	fmt.Println("Flushing linko pf anchor...")
	slog.Info("flushing pf anchor", "anchor", anchorName)

	ctx1, cancel1 := context.WithTimeout(context.Background(), timeout)
	defer cancel1()

	flushCmd := exec.CommandContext(ctx1, "sudo", "pfctl", "-a", anchorName, "-F", "all")
	var flushStderr bytes.Buffer
	flushCmd.Stderr = &flushStderr
	if err := flushCmd.Run(); err != nil {
		slog.Error("failed to flush pf anchor", "anchor", anchorName, "error", err, "stderr", flushStderr.String())
		fmt.Printf("  FAILED: %v\n", err)
		hasError = true
	} else {
		fmt.Println("  OK: pf anchor flushed")
	}

	// Step 2: Disable pf
	fmt.Println("Disabling pf...")

	ctx2, cancel2 := context.WithTimeout(context.Background(), timeout)
	defer cancel2()

	disableCmd := exec.CommandContext(ctx2, "sudo", "pfctl", "-d")
	var disableStderr bytes.Buffer
	disableCmd.Stderr = &disableStderr
	if err := disableCmd.Run(); err != nil {
		slog.Error("failed to disable pf", "error", err, "stderr", disableStderr.String())
		fmt.Printf("  FAILED: %v\n", err)
		hasError = true
	} else {
		fmt.Println("  OK: pf disabled")
	}

	// Step 3: Remove config file
	fmt.Println("Removing config file...")

	rmCmd := exec.Command("sudo", "rm", "-f", confPath)
	if err := rmCmd.Run(); err != nil {
		slog.Error("failed to remove config file", "path", confPath, "error", err)
		fmt.Printf("  FAILED: %v\n", err)
		hasError = true
	} else {
		fmt.Println("  OK: " + confPath + " removed")
	}

	// Step 4: Remove anchor line from /etc/pf.conf
	fmt.Println("Removing anchor line from /etc/pf.conf...")

	anchorLine := fmt.Sprintf(`load anchor "%s" from "%s"`, anchorName, confPath)
	if err := removeLineFromPfConf(anchorLine); err != nil {
		slog.Error("failed to remove anchor line", "error", err)
		fmt.Printf("  FAILED: %v\n", err)
		hasError = true
	} else {
		fmt.Println("  OK: anchor line removed")
	}

	// Summary
	fmt.Println()
	if !hasError {
		fmt.Println("Cleanup complete. Network connectivity should be restored.")
	} else {
		fmt.Println("Cleanup completed with errors. Check the output above.")
	}
}

func removeLineFromPfConf(anchorLine string) error {
	data, err := os.ReadFile("/etc/pf.conf")
	if err != nil {
		return fmt.Errorf("failed to read /etc/pf.conf: %w", err)
	}

	content := string(data)
	if !strings.Contains(content, anchorLine) {
		return nil // nothing to remove
	}

	var lines []string
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) != anchorLine {
			lines = append(lines, line)
		}
	}
	newContent := strings.Join(lines, "\n")

	cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("cat > /etc/pf.conf << 'EOF'\n%s\nEOF", newContent))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to write /etc/pf.conf: %w\nstderr: %s", err, stderr.String())
	}
	return nil
}
