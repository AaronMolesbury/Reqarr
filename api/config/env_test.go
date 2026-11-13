package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	const prefix = "REQARR_ENV_TEST_"
	for _, suffix := range []string{"URL", "KEY", "QUOTED", "EMPTY", "EXISTING", "EXISTING_EMPTY"} {
		key := prefix + suffix
		old, existed := os.LookupEnv(key)
		os.Unsetenv(key)
		t.Cleanup(func() {
			if existed {
				os.Setenv(key, old)
			} else {
				os.Unsetenv(key)
			}
		})
	}
	t.Setenv(prefix+"EXISTING", "from-process")
	t.Setenv(prefix+"EXISTING_EMPTY", "")
	path := filepath.Join(t.TempDir(), "settings.txt")
	content := "\ufeff# Local settings\r\n\r\n" +
		prefix + "URL = https://service.example.com\r\n" +
		prefix + "KEY=abc=123#$literal\r\n" +
		prefix + "QUOTED=' value with spaces '\r\n" +
		prefix + "EMPTY=\"\"\r\n" +
		prefix + "EXISTING=from-file\r\n" +
		prefix + "EXISTING_EMPTY=from-file\r\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := LoadEnvFile(path); err != nil {
		t.Fatal(err)
	}
	for suffix, want := range map[string]string{"URL": "https://service.example.com", "KEY": "abc=123#$literal", "QUOTED": " value with spaces ", "EMPTY": "", "EXISTING": "from-process", "EXISTING_EMPTY": ""} {
		if got, exists := os.LookupEnv(prefix + suffix); !exists || got != want {
			t.Errorf("%s = %q (exists %v), want %q", suffix, got, exists, want)
		}
	}
}

func TestLoadEnvFileMissingAndInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.txt")
	if err := LoadEnvFile(path); err != nil {
		t.Fatalf("missing file: %v", err)
	}
	for _, content := range []string{"secret-without-assignment", "1INVALID=secret", "KEY='secret"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		err := LoadEnvFile(path)
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("expected a safe error, got %v", err)
		}
	}
}
