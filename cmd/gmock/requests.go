package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/spf13/cobra"
)

// newRequestsCmd creates the `gmock requests` subcommand for viewing the request log.
func newRequestsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "requests",
		Short: "View the request log on a running server",
		RunE: func(cmd *cobra.Command, args []string) error {
			adminURL, _ := cmd.Flags().GetString("admin-url")
			filter, _ := cmd.Flags().GetString("filter")

			u, err := url.Parse(adminURL + "/__admin/requests")
			if err != nil {
				return fmt.Errorf("invalid admin URL: %w", err)
			}
			if filter != "" {
				q := u.Query()
				q.Set("filter", filter)
				u.RawQuery = q.Encode()
			}

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
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, body, "", "  "); err == nil {
				fmt.Fprintln(cmd.OutOrStdout(), pretty.String())
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), string(body))
			}
			return nil
		},
	}

	cmd.Flags().String("admin-url", "http://localhost:8080",
		"Base URL of the running gmock server admin API")
	cmd.Flags().String("filter", "", "Filter requests: 'matched', 'unmatched', or empty for all")

	return cmd
}
