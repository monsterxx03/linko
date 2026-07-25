package main

import (
	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove linko firewall rules",
	Long: `Cleanup removes all linko firewall rules (nftables on Linux, pf on macOS).

This command is useful for recovering from unexpected crashes or SIGKILL signals
where linko did not get a chance to clean up its firewall rules gracefully.

If linko was killed with SIGKILL (kill -9), or the process crashed, the redirect
rules may still be active, causing network traffic to be redirected to a proxy
that is no longer running. Running 'linko cleanup' will restore normal network
connectivity.

Requires root privileges (sudo).`,
	Run: runCleanup,
}
