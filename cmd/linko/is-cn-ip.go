package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/monsterxx03/linko/pkg/ipdb"
	"github.com/spf13/cobra"
)

var isCnIPCmd = &cobra.Command{
	Use:   "is-cn-ip <ip>",
	Short: "Check if an IP is a China IP address",
	Long:  "Check if the given IP address falls within China IP ranges",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := ipdb.LoadChinaIPRanges(); err != nil {
			slog.Error("failed to load China IP ranges", "error", err)
			os.Exit(1)
		}
		ip := args[0]
		isCN := ipdb.IsChinaIP(ip)
		if isCN {
			fmt.Printf("%s is a China IP address\n", ip)
		} else {
			fmt.Printf("%s is NOT a China IP address\n", ip)
		}
	},
}
