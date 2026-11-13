package config

import (
	"strings"
	"testing"
)

func TestLoadServiceSettings(t *testing.T) {
	for _, key := range []string{"SEERR_URL", "SEERR_API_KEY", "RADARR_URL", "RADARR_API_KEY", "SONARR_URL", "SONARR_API_KEY", "HTTP_ADDR", "STATIC_DIR", "NOTIFICATIONS_ENABLED"} {
		t.Setenv(key, "")
	}
	if _, err := Load(); err == nil {
		t.Fatal("Seerr settings must be required")
	}
	t.Setenv("SEERR_URL", "https://seerr.example.com/")
	t.Setenv("SEERR_API_KEY", "test-key")
	c, err := Load()
	if err != nil || c.Seerr.URL != "https://seerr.example.com" || c.HTTPAddr != "127.0.0.1:8080" || c.Radarr.URL != "" || c.Notifications.Enabled {
		t.Fatalf("unexpected defaults: %v", err)
	}
	for _, service := range []string{"RADARR", "SONARR"} {
		t.Run(service, func(t *testing.T) {
			t.Setenv(service+"_URL", "https://service.example.com")
			if _, err := Load(); err == nil {
				t.Fatal("URL without key must fail")
			}
			t.Setenv(service+"_API_KEY", "test-key")
			if _, err := Load(); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, value := range []string{"ftp://service.example.com", "https://user:secret@service.example.com", "https://service.example.com?key=secret", "https://service.example.com#fragment"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("SEERR_URL", value)
			if _, err := Load(); err == nil || strings.Contains(err.Error(), value) {
				t.Fatal("expected validation error without populated URL")
			}
		})
	}
}
