package main

import (
	"fmt"

	"github.com/monsterxx03/linko/pkg/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("linko version %s\n", version.Version)
		if version.Commit != "" {
			fmt.Printf("commit: %s\n", version.Commit)
		}
		if version.Date != "" {
			fmt.Printf("built: %s\n", version.Date)
		}
	},
}
