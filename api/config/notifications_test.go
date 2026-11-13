package config

import (
	"strings"
	"testing"
	"time"
)

func TestNotificationConfig(t *testing.T) {
	for _, key := range []string{"NOTIFICATIONS_ENABLED", "NOTIFICATION_DB", "NOTIFICATION_INTERVAL", "NTFY_URL", "NTFY_TOPIC", "NTFY_TOKEN", "NTFY_USERNAME", "NTFY_PASSWORD"} {
		t.Setenv(key, "")
	}
	c, err := LoadNotifications()
	if err != nil || c.Enabled {
		t.Fatal("notifications must be opt-in")
	}
	t.Setenv("NOTIFICATIONS_ENABLED", "true")
	t.Setenv("NTFY_URL", "https://notify.example.com")
	t.Setenv("NTFY_TOPIC", "reqarr")
	t.Setenv("NTFY_USERNAME", "tester")
	t.Setenv("NTFY_PASSWORD", "password")
	if c, err = LoadNotifications(); err != nil || c.Interval != 15*time.Second {
		t.Fatalf("valid basic auth config: %v", err)
	}
	for _, test := range []struct{ key, value string }{
		{"NTFY_URL", "http://notify.example.com"},
		{"NTFY_URL", "https://secret:password@notify.example.com"},
		{"NTFY_TOPIC", "reqarr?token=secret"},
		{"NTFY_TOKEN", "conflicting-token"},
		{"NOTIFICATION_INTERVAL", "0s"},
	} {
		t.Run(test.key+test.value, func(t *testing.T) {
			t.Setenv(test.key, test.value)
			if _, err := LoadNotifications(); err == nil || strings.Contains(err.Error(), test.value) {
				t.Fatal("expected safe config error")
			}
		})
	}
}
