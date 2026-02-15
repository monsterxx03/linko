package main

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/monsterxx03/linko/pkg/config"
	"github.com/spf13/cobra"
)

var configPathFlag string

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Generate default configuration file",
	Long:  "Generate a default configuration file at the specified path",
	Run: func(cmd *cobra.Command, args []string) {
		if err := config.GenerateConfig(configPathFlag); err != nil {
			slog.Error("failed to generate config", "error", err)
			os.Exit(1)
		}
		slog.Info("default config generated", "path", configPathFlag)
	},
}

func init() {
	configCmd.Flags().StringVarP(&configPathFlag, "output", "o", filepath.Join(config.GetConfigDir(), "linko.yaml"), "Output configuration file path")
}
