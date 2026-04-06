package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/charmbracelet/crush/internal/router"
	"github.com/spf13/cobra"
)

var (
	routerAddr      string
	routerLocalPool string
	routerCloudAPI  string
)

func init() {
	routerCmd.Flags().StringVarP(&routerAddr, "addr", "a", ":8000", "Address to listen on")
	routerCmd.Flags().StringVar(&routerLocalPool, "local-pool", "http://localhost:11434", "Local pool URL (e.g. Android Nginx cluster)")
	routerCmd.Flags().StringVar(&routerCloudAPI, "cloud-api", "https://api.anthropic.com", "Cloud API URL")
	rootCmd.AddCommand(routerCmd)
}

var routerCmd = &cobra.Command{
	Use:   "router",
	Short: "Start the Hybrid Stream Routing Proxy",
	RunE: func(cmd *cobra.Command, args []string) error {
		r := router.New(router.Config{
			LocalPool: routerLocalPool,
			CloudAPI:  routerCloudAPI,
		})

		srv := &http.Server{
			Addr:    routerAddr,
			Handler: r,
		}

		errch := make(chan error, 1)
		sigch := make(chan os.Signal, 1)
		sigs := []os.Signal{os.Interrupt}
		sigs = append(sigs, addSignals(sigs)...)
		signal.Notify(sigch, sigs...)

		slog.Info("Starting Crush Router...", "addr", routerAddr, "local", routerLocalPool, "cloud", routerCloudAPI)

		go func() {
			errch <- srv.ListenAndServe()
		}()

		var err error
		select {
		case <-sigch:
			slog.Info("Received interrupt signal...")
		case err = <-errch:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("Router error", "error", err)
				return fmt.Errorf("router error: %v", err)
			}
		}

		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
		defer cancel()

		slog.Info("Shutting down router...")

		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("Failed to shutdown router", "error", err)
			return fmt.Errorf("failed to shutdown router: %v", err)
		}

		return nil
	},
}
