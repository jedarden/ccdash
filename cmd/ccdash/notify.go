package main

import (
	"fmt"
	"os"
	"time"

	"github.com/jedarden/ccdash/internal/config"
	"github.com/jedarden/ccdash/internal/notify"
)

// runTestNotify loads the user's config, sends one synthetic notification, and
// returns a process exit code. It deliberately does not initialize the TUI.
func runTestNotify() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stdout, "Notification test failed: %v\n", err)
		return 1
	}

	if !cfg.Notify.Enabled {
		fmt.Fprintln(os.Stdout, "Notification test failed: notifications are disabled in config")
		return 1
	}
	if cfg.Notify.WebhookURL == "" {
		fmt.Fprintln(os.Stdout, "Notification test failed: notify.webhook_url is not configured")
		return 1
	}

	payload := &notify.Payload{
		SessionName:  "ccdash-test",
		ProjectDir:   "/test/project",
		IdleDuration: 30 * time.Second,
		Timestamp:    time.Now(),
	}
	result := notify.NewClient(cfg.Notify.WebhookURL, cfg.Notify.Enabled).SendTest(payload)

	if result.Success {
		fmt.Fprintf(os.Stdout, "Notification test succeeded: HTTP status %d\n", result.StatusCode)
		return 0
	}

	if result.StatusCode > 0 {
		if result.Error != "" {
			fmt.Fprintf(os.Stdout, "Notification test failed: HTTP status %d: %s\n", result.StatusCode, result.Error)
		} else {
			fmt.Fprintf(os.Stdout, "Notification test failed: HTTP status %d\n", result.StatusCode)
		}
		return 1
	}

	if result.Error == "" {
		result.Error = "unknown webhook error"
	}
	fmt.Fprintf(os.Stdout, "Notification test failed: %s\n", result.Error)
	return 1
}
