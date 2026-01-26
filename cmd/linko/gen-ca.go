package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/monsterxx03/linko/pkg/mitm"
	"github.com/spf13/cobra"
)

var genCaOutputDir string

var genCaCmd = &cobra.Command{
	Use:   "gen-ca",
	Short: "Generate CA certificate and private key for MITM proxy",
	Long:  "Generate a self-signed CA certificate and private key for HTTPS MITM inspection",
	Run: func(cmd *cobra.Command, args []string) {
		if genCaOutputDir == "" {
			genCaOutputDir = "certs"
		}

		caCertPath := filepath.Join(genCaOutputDir, "ca.crt")
		caKeyPath := filepath.Join(genCaOutputDir, "ca.key")

		slog.Info("generating CA certificate", "output_dir", genCaOutputDir)

		if err := mitm.CreateCAOnly(caCertPath, caKeyPath, 10*365*24*time.Hour); err != nil {
			slog.Error("failed to generate CA", "error", err)
			os.Exit(1)
		}

		slog.Info("CA certificate generated successfully",
			"cert_path", caCertPath,
			"key_path", caKeyPath,
		)
		fmt.Printf("CA certificate generated:\n  %s\n  %s\n", caCertPath, caKeyPath)
		fmt.Println("\nPlease install the CA certificate in your system/browser trust store.")
	},
}

func init() {
	genCaCmd.Flags().StringVarP(&genCaOutputDir, "output", "o", "", "Output directory for CA files (default: ./certs)")
}
