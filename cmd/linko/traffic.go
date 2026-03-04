package main

import (
	"github.com/monsterxx03/linko/cmd/linko/traffic"
	"github.com/spf13/cobra"
)

var trafficCmd = &cobra.Command{
	Use:   "traffic",
	Short: "MITM traffic monitor TUI",
	Long:  `Real-time MITM traffic monitor using a TUI interface. Connects to the local Admin API and displays traffic events in real-time.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL, err := cmd.Flags().GetString("server")
		if err != nil {
			return err
		}
		return traffic.Run(serverURL)
	},
}

func init() {
	trafficCmd.Flags().StringP("server", "s", "http://localhost:9810/api/mitm/traffic/sse", "SSE endpoint URL")
	rootCmd.AddCommand(trafficCmd)
}
