package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/spf13/cobra"
)

// newReportCmd creates the `gmock report` subcommand for exporting chaos
// evidence (JUnit XML or JSON) from a running server.
func newReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Export chaos evidence from a running server (JUnit XML or JSON)",
		RunE: func(cmd *cobra.Command, args []string) error {
			adminURL, _ := cmd.Flags().GetString("admin-url")
			format, _ := cmd.Flags().GetString("format")

			u, err := url.Parse(adminURL + "/__admin/report")
			if err != nil {
				return fmt.Errorf("invalid admin URL: %w", err)
			}
			q := u.Query()
			q.Set("format", format)
			u.RawQuery = q.Encode()

			resp, err := commonClient.Get(u.String())
			if err != nil {
				return fmt.Errorf("connect to server: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode >= 400 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
			}

			body, _ := io.ReadAll(resp.Body)
			if format == "json" {
				var pretty bytes.Buffer
				if err := json.Indent(&pretty, body, "", "  "); err == nil {
					body = pretty.Bytes()
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(body))
			return nil
		},
	}

	cmd.Flags().String("admin-url", "http://localhost:8080",
		"Base URL of the running gmock server admin API")
	cmd.Flags().String("format", "json", "Report format: 'json' or 'junit'")

	return cmd
}
