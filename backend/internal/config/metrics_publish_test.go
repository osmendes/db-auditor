package config

import (
	"os"
	"strings"
	"testing"
)

func TestComposeDoesNotPublishMetricsPort(t *testing.T) {
	body, err := os.ReadFile("../../../compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "9090:9090") {
		t.Fatal("metrics port is published on the host; scrape api:9090 inside the compose network")
	}
}
