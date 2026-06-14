package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/sunny809/gochaos/pkg/gmock"
)

// newStartCmd creates the `gmock start` subcommand.
func newStartCmd() *cobra.Command {
	var (
		port        int
		adminPort   int
		stubFiles   []string
		proxyURL    string
		recordMode  bool
		verbose     bool
		maxRequests int
	)

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the gmock server",
		Long:  `Start the gmock HTTP server. The server runs until interrupted (Ctrl-C).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := []gmock.Option{
				gmock.WithPort(port),
				gmock.WithMaxRequests(maxRequests),
			}
			if adminPort > 0 {
				opts = append(opts, gmock.WithAdminPort(adminPort))
			}
			if len(stubFiles) > 0 {
				opts = append(opts, gmock.WithStubFiles(stubFiles...))
			}
			if proxyURL != "" {
				opts = append(opts, gmock.WithProxyURL(proxyURL))
			}
			if recordMode {
				opts = append(opts, gmock.WithRecordMode())
			}
			if verbose {
				opts = append(opts, gmock.WithVerbose())
			}
			cors, _ := cmd.Flags().GetBool("cors")
			if cors {
				opts = append(opts, gmock.WithCORSEnabled())
			}

			server := gmock.NewServer(opts...)
			if err := server.Start(); err != nil {
				return fmt.Errorf("start server: %w", err)
			}

			fmt.Printf("gmock listening at %s\n", server.URL())
			if server.AdminURL() != server.URL() {
				fmt.Printf("admin API at %s/__admin/\n", server.AdminURL())
			} else {
				fmt.Printf("admin API at %s/__admin/\n", server.URL())
			}
			fmt.Println("Press Ctrl-C to stop")

			// Wait for interrupt
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			<-sigCh

			fmt.Println("\nShutting down...")
			return server.Stop()
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8080, "HTTP port to listen on")
	cmd.Flags().IntVar(&adminPort, "admin-port", 0, "Separate admin API port (0 = same as main port)")
	cmd.Flags().StringSliceVarP(&stubFiles, "stubs", "s", nil, "Stub files to load (.yaml or .json, can be specified multiple times)")
	cmd.Flags().StringVar(&proxyURL, "proxy-url", "", "Upstream URL for proxy fallback")
	cmd.Flags().BoolVar(&recordMode, "record", false, "Enable recording of proxied requests")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable debug logging")
	cmd.Flags().Bool("cors", false, "Enable CORS with default settings (allow all origins)")
	cmd.Flags().IntVar(&maxRequests, "max-requests", 1000, "Maximum number of requests to keep in the log")

	return cmd
}