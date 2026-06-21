package main

import (
	"github.com/spf13/cobra"
)

// Build-time variables set via -ldflags "-X main.version=..."
var (
	version   = "dev"
	commit    = ""
	buildDate = ""
)

// newRootCmd creates the root command for the gmock CLI.
func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gmock",
		Short: "gmock — a Go-native HTTP mock server",
		Long: `gmock is a lightweight, embeddable HTTP mock server inspired by WireMock.

Use it standalone as a CLI binary, or embed it in your Go tests as a library.

Examples:
  # Start a server on port 8080 with no stubs (use admin API to add them)
  gmock start --port 8080

  # Start with stubs loaded from YAML/JSON files
  gmock start --port 8080 --stubs ./stubs.yaml

  # Use the admin API while the server is running
  gmock stub list
  gmock stub create ./new-stub.json
  gmock reset`,
		Version: version,
	}

	cmd.AddCommand(newStartCmd())
	cmd.AddCommand(newStubCmd())
	cmd.AddCommand(newResetCmd())
	cmd.AddCommand(newRequestsCmd())

	return cmd
}
