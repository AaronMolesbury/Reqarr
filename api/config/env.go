package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

// LoadEnvFile reads optional local configuration without overriding process
// variables. Values are literal: no interpolation, escapes, or inline comments.
func LoadEnvFile(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open .env: %w", err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if line == 1 {
			text = strings.TrimSpace(strings.TrimPrefix(text, "\ufeff"))
		}
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, value, found := strings.Cut(text, "=")
		key = strings.TrimSpace(key)
		if !found || !validEnvKey(key) {
			return fmt.Errorf("invalid .env assignment on line %d", line)
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'") {
			if len(value) < 2 || value[len(value)-1] != value[0] {
				return fmt.Errorf("invalid .env quoted value on line %d", line)
			}
			value = value[1 : len(value)-1]
		}
		values[key] = value
	}
	if scanner.Err() != nil {
		// Scanner errors can contain input; never include configuration values.
		return errors.New("could not read .env")
	}
	for key, value := range values {
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return errors.New("could not apply .env configuration")
		}
	}
	return nil
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i, char := range key {
		if char != '_' && !(char >= 'A' && char <= 'Z') && !(char >= 'a' && char <= 'z') && !(i > 0 && char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}
