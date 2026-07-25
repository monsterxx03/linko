//go:build linux

package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
	"github.com/monsterxx03/linko/pkg/proxy"
)

const nftConfPath = "/etc/nftables/linko.conf"

func runCleanup(cmd *cobra.Command, args []string) {
	const timeout = 10 * time.Second
	var hasError bool

	// Step 1: Delete the linko nftables table (removes all chains, sets, rules atomically)
	fmt.Println("Deleting linko nftables table...")
	slog.Info("deleting nftables table", "table", "ip linko")

	ctx1, cancel1 := context.WithTimeout(context.Background(), timeout)
	defer cancel1()

	deleteCmd := exec.CommandContext(ctx1, "sudo", "nft", "delete", "table", "ip", "linko")
	var deleteStderr bytes.Buffer
	deleteCmd.Stderr = &deleteStderr
	if err := deleteCmd.Run(); err != nil {
		slog.Error("failed to delete nftables table (may be already gone)", "error", err, "stderr", deleteStderr.String())
		fmt.Printf("  FAILED: %v\n", err)
		hasError = true
	} else {
		fmt.Println("  OK: nftables table deleted")
	}

	// Step 2: Remove config file
	fmt.Println("Removing config file...")

	rmCmd := exec.Command("sudo", "rm", "-f", nftConfPath)
	if err := rmCmd.Run(); err != nil {
		slog.Error("failed to remove config file", "path", nftConfPath, "error", err)
		fmt.Printf("  FAILED: %v\n", err)
		hasError = true
	} else {
		fmt.Println("  OK: " + nftConfPath + " removed")
	}

	// Step 3: Clean up cgroup (harmless if linko is still running — removal
	// will fail with EBUSY, which is expected)
	fmt.Println("Cleaning up linko cgroup...")
	proxy.CleanupLinkoCgroup()
	fmt.Println("  OK: cgroup cleanup attempted")

	// Summary
	fmt.Println()
	if !hasError {
		fmt.Println("Cleanup complete. Network connectivity should be restored.")
	} else {
		fmt.Println("Cleanup completed with errors. Check the output above.")
	}
}
