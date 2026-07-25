//go:build darwin

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

const pfAnchorName = "com.apple/linko"
const pfConfPath = "/etc/pf.linko.conf"

func runCleanup(cmd *cobra.Command, args []string) {
	const timeout = 10 * time.Second
	var hasError bool

	// Step 1: Flush the linko pf anchor
	fmt.Println("Flushing linko pf anchor...")
	slog.Info("flushing pf anchor", "anchor", pfAnchorName)

	ctx1, cancel1 := context.WithTimeout(context.Background(), timeout)
	defer cancel1()

	flushCmd := exec.CommandContext(ctx1, "sudo", "pfctl", "-a", pfAnchorName, "-F", "all")
	var flushStderr bytes.Buffer
	flushCmd.Stderr = &flushStderr
	if err := flushCmd.Run(); err != nil {
		slog.Error("failed to flush pf anchor", "anchor", pfAnchorName, "error", err, "stderr", flushStderr.String())
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

	rmCmd := exec.Command("sudo", "rm", "-f", pfConfPath)
	if err := rmCmd.Run(); err != nil {
		slog.Error("failed to remove config file", "path", pfConfPath, "error", err)
		fmt.Printf("  FAILED: %v\n", err)
		hasError = true
	} else {
		fmt.Println("  OK: " + pfConfPath + " removed")
	}

	// Step 4: Remove anchor line from /etc/pf.conf
	fmt.Println("Removing anchor line from /etc/pf.conf...")

	anchorLine := fmt.Sprintf(`load anchor "%s" from "%s"`, pfAnchorName, pfConfPath)
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
	for line := range strings.SplitSeq(content, "\n") {
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
