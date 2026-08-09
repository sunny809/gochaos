package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/sunny809/gochaos/config"
)

// commonClient is the HTTP client used by all admin CLI commands.
var (
	commonClient   = &http.Client{}
	commonAdminURL string
)

// newStubCmd creates the `gmock stub` subcommand.
func newStubCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stub",
		Short: "Manage stubs on a running gmock server",
	}

	cmd.PersistentFlags().StringVar(&commonAdminURL, "admin-url", "http://localhost:8080",
		"Base URL of the running gmock server admin API")

	cmd.AddCommand(newStubListCmd())
	cmd.AddCommand(newStubCreateCmd())
	cmd.AddCommand(newStubDeleteCmd())
	cmd.AddCommand(newStubGetCmd())

	return cmd
}

func newStubListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all stubs on the running server",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := commonClient.Get(commonAdminURL + "/__admin/mappings")
			if err != nil {
				return fmt.Errorf("connect to server: %w", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("read response body: %w", err)
			}
			// Pretty-print JSON
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, body, "", "  "); err == nil {
				fmt.Fprintln(cmd.OutOrStdout(), pretty.String())
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), string(body))
			}
			return nil
		},
	}
}

func newStubCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <file>",
		Short: "Create a stub from a JSON or YAML file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			stubs, err := config.LoadStubsFromFile(args[0])
			if err != nil {
				return err
			}
			if len(stubs) == 0 {
				return fmt.Errorf("no stubs found in %s", args[0])
			}

			for i, def := range stubs {
				if err := postOneStub(cmd, i, def); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// postOneStub marshals a single stub, POSTs it to the admin API, and prints
// the created stub ID. Returns an error with stub index context.
func postOneStub(cmd *cobra.Command, i int, def interface{}) error {
	data, err := json.Marshal(def)
	if err != nil {
		return fmt.Errorf("stub %d: marshal: %w", i, err)
	}
	resp, err := commonClient.Post(
		commonAdminURL+"/__admin/mappings",
		"application/json",
		bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("stub %d: post: %w", i, err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return fmt.Errorf("stub %d: read body: %w", i, err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("stub %d: server returned %d: %s", i, resp.StatusCode, string(body))
	}

	var created map[string]interface{}
	if err := json.Unmarshal(body, &created); err != nil {
		return fmt.Errorf("stub %d: unmarshal response: %w", i, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "created stub: %v\n", created["id"])
	return nil
}

func newStubGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a stub by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := commonClient.Get(commonAdminURL + "/__admin/mappings/" + url.PathEscape(args[0]))
			if err != nil {
				return fmt.Errorf("connect to server: %w", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("read response body: %w", err)
			}
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, body, "", "  "); err == nil {
				fmt.Fprintln(cmd.OutOrStdout(), pretty.String())
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), string(body))
			}
			if resp.StatusCode >= 400 {
				return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
			}
			return nil
		},
	}
}

func newStubDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [id]",
		Short: "Delete a stub by ID (or --all to delete all)",
		RunE: func(cmd *cobra.Command, args []string) error {
			deleteAll, _ := cmd.Flags().GetBool("all")
			u := commonAdminURL + "/__admin/mappings"
			if !deleteAll {
				if len(args) != 1 {
					return fmt.Errorf("provide a stub ID or use --all")
				}
				u += "/" + url.PathEscape(args[0])
			}

			req, err := http.NewRequest(http.MethodDelete, u, nil)
			if err != nil {
				return fmt.Errorf("build request: %w", err)
			}
			resp, err := commonClient.Do(req)
			if err != nil {
				return fmt.Errorf("connect to server: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode >= 400 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
			}
			fmt.Fprintln(cmd.OutOrStdout(), "deleted")
			return nil
		},
	}
	cmd.Flags().Bool("all", false, "Delete all stubs")
	return cmd
}
