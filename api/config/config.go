package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Service struct {
	URL string
	Key string
}

type Config struct {
	HTTPAddr      string
	StaticDir     string
	Seerr         Service
	Radarr        Service
	Sonarr        Service
	Notifications Notifications
}

// Load reads process settings only. Loading a local file is an explicit startup step.
func Load() (Config, error) {
	var c Config
	var err error
	if c.Seerr, err = loadService("SEERR", true); err != nil {
		return c, err
	}
	if c.Radarr, err = loadService("RADARR", false); err != nil {
		return c, err
	}
	if c.Sonarr, err = loadService("SONARR", false); err != nil {
		return c, err
	}
	c.HTTPAddr = os.Getenv("HTTP_ADDR")
	c.StaticDir = os.Getenv("STATIC_DIR")
	if c.HTTPAddr == "" {
		c.HTTPAddr = "127.0.0.1:8080"
	}
	c.Notifications, err = LoadNotifications()
	return c, err
}

func loadService(prefix string, required bool) (Service, error) {
	c := Service{URL: strings.TrimRight(os.Getenv(prefix+"_URL"), "/"), Key: os.Getenv(prefix + "_API_KEY")}
	if required && (c.URL == "" || c.Key == "") {
		return c, fmt.Errorf("%s_URL and %s_API_KEY are required", prefix, prefix)
	}
	if (c.URL == "") != (c.Key == "") {
		return c, fmt.Errorf("%s_URL and %s_API_KEY must both be set, or both omitted", prefix, prefix)
	}
	if c.URL != "" {
		parsed, err := url.Parse(c.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return c, fmt.Errorf("%s_URL must be an http(s) base URL without credentials, query or fragment", prefix)
		}
	}
	return c, nil
}
