package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/sunny809/gochaos/config"
)

// commonClient is the HTTP client used by all admin CLI commands.
var commonClient = &http.Client{}

// commonAdminURL holds the base URL passed via --admin-url.
var commonAdminURL string

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

			body, _ := io.ReadAll(resp.Body)
			// Pretty-print JSON
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, body, "", "  "); err == nil {
				fmt.Println(pretty.String())
			} else {
				fmt.Println(string(body))
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
				data, _ := json.Marshal(def)
				resp, err := commonClient.Post(
					commonAdminURL+"/__admin/mappings",
					"application/json",
					bytes.NewReader(data))
				if err != nil {
					return fmt.Errorf("stub %d: post: %w", i, err)
				}
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()

				if resp.StatusCode >= 400 {
					return fmt.Errorf("stub %d: server returned %d: %s", i, resp.StatusCode, string(body))
				}

				var created map[string]interface{}
				_ = json.Unmarshal(body, &created)
				fmt.Printf("created stub: %v\n", created["id"])
			}
			return nil
		},
	}
}

func newStubGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a stub by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := commonClient.Get(commonAdminURL + "/__admin/mappings/" + args[0])
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
			if resp.StatusCode >= 400 {
				os.Exit(1)
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
			url := commonAdminURL + "/__admin/mappings"
			if !deleteAll {
				if len(args) != 1 {
					return fmt.Errorf("provide a stub ID or use --all")
				}
				url += "/" + args[0]
			}

			req, _ := http.NewRequest(http.MethodDelete, url, nil)
			resp, err := commonClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode >= 400 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
			}
			fmt.Println("deleted")
			return nil
		},
	}
	cmd.Flags().Bool("all", false, "Delete all stubs")
	return cmd
}

