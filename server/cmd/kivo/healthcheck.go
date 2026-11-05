package main

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/mmcdole/kivo/internal/config"
)

// healthcheck queries the local server's /healthz, for the container
// image's HEALTHCHECK.
func healthcheck(configPath string) error {
	boot, err := config.LoadBootstrap(configPath)
	if err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(boot.Listen[0])
	if err != nil {
		return err
	}
	scheme := "http"
	if boot.TLSCert != "" {
		scheme = "https"
	}
	c := http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get(fmt.Sprintf("%s://127.0.0.1:%s%s/healthz", scheme, port, boot.BasePath))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: %s", resp.Status)
	}
	return nil
}
