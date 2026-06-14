package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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

			url := adminURL + "/__admin/requests"
			if filter != "" {
				url += "?filter=" + filter
			}

			resp, err := http.Get(url)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, body, "", "  "); err == nil {
				fmt.Println(pretty.String())
			} else {
				fmt.Println(string(body))
			}
			return nil
		},
	}

	cmd.Flags().String("admin-url", "http://localhost:8080",
		"Base URL of the running gmock server admin API")
	cmd.Flags().String("filter", "", "Filter requests: 'matched', 'unmatched', or empty for all")

	return cmd
}
