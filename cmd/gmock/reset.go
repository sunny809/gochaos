package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// newResetCmd creates the `gmock reset` subcommand.
func newResetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Reset all stubs and request log on the running server",
		RunE: func(cmd *cobra.Command, args []string) error {
			adminURL, _ := cmd.Flags().GetString("admin-url")
			resp, err := commonClient.Post(adminURL+"/__admin/reset", "application/json", nil)
			if err != nil {
				return fmt.Errorf("connect to server: %w", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("read response body: %w", err)
			}
			if resp.StatusCode >= 400 {
				return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
			}
			fmt.Fprintln(cmd.OutOrStdout(), "server state reset")
			return nil
		},
	}

	cmd.Flags().String("admin-url", "http://localhost:8080",
		"Base URL of the running gmock server admin API")

	return cmd
}
