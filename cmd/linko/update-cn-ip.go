package main

import (
	"log/slog"
	"os"

	"github.com/monsterxx03/linko/pkg/ipdb"
	"github.com/spf13/cobra"
)

var updateCnIPCmd = &cobra.Command{
	Use:   "update-cn-ip",
	Short: "Download China IP ranges from APNIC",
	Long:  "Fetch the latest China IP address ranges from APNIC and save to data directory",
	Run: func(cmd *cobra.Command, args []string) {
		slog.Info("fetching China IP ranges from APNIC...")
		if err := ipdb.FetchChinaIPRanges(); err != nil {
			slog.Error("failed to fetch China IP ranges", "error", err)
			os.Exit(1)
		}
		slog.Info("China IP ranges updated successfully", "output_dir", "pkg/ipdb/china_ip_ranges.go")
	},
}
