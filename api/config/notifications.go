package config

import (
	"errors"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Notifications struct {
	Enabled                               bool
	URL, Topic, Token, Username, Password string
	Database                              string
	Interval                              time.Duration
}

func LoadNotifications() (Notifications, error) {
	c := Notifications{Database: os.Getenv("NOTIFICATION_DB"), Interval: 15 * time.Second}
	if c.Database == "" {
		c.Database = "data/reqarr.db"
	}
	if value := os.Getenv("NOTIFICATIONS_ENABLED"); value != "" {
		var err error
		c.Enabled, err = strconv.ParseBool(value)
		if err != nil {
			return c, errors.New("NOTIFICATIONS_ENABLED must be true or false")
		}
	}
	if !c.Enabled {
		return c, nil
	}
	c.URL = strings.TrimRight(os.Getenv("NTFY_URL"), "/")
	c.Topic, c.Token = os.Getenv("NTFY_TOPIC"), os.Getenv("NTFY_TOKEN")
	c.Username, c.Password = os.Getenv("NTFY_USERNAME"), os.Getenv("NTFY_PASSWORD")
	u, err := url.Parse(c.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return c, errors.New("NTFY_URL must be an HTTPS base URL without credentials, query or fragment")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`).MatchString(c.Topic) {
		return c, errors.New("NTFY_TOPIC must contain 1-64 letters, digits, underscores or hyphens")
	}
	if (c.Username == "") != (c.Password == "") || (c.Token != "" && c.Username != "") {
		return c, errors.New("use either NTFY_TOKEN or both NTFY_USERNAME and NTFY_PASSWORD")
	}
	if c.Token == "" && c.Username == "" {
		return c, errors.New("ntfy authentication is required when notifications are enabled")
	}
	if value := os.Getenv("NOTIFICATION_INTERVAL"); value != "" {
		c.Interval, err = time.ParseDuration(value)
		if err != nil || c.Interval < time.Second {
			return c, errors.New("NOTIFICATION_INTERVAL must be a duration of at least 1s")
		}
	}
	return c, nil
}
