package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"reqarr/config"
	"reqarr/dashboard"
	"reqarr/notifications"
	"reqarr/server"
	"reqarr/storage"
	"reqarr/upstream"
)

func main() {
	if err := config.LoadEnvFile(".env"); err != nil {
		log.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	client := upstream.NewHTTPClient()
	stop, cancelStop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelStop()
	missing := &dashboard.MissingTracker{}
	collector := dashboard.NewCollector(client, cfg.Seerr, cfg.Radarr, cfg.Sonarr)
	source := &dashboard.Source{
		Missing:       missing,
		Ctx:           stop,
		Cooldown:      90 * time.Second,
		ErrorCooldown: 90 * time.Second,
		Collect:       collector.Collect,
	}
	handler := server.NewHandler(client, cfg.Seerr, source.Get)
	if cfg.StaticDir != "" {
		if _, err := os.Stat(cfg.StaticDir + "/index.html"); err != nil {
			log.Fatal("STATIC_DIR must contain the built client index.html")
		}
		handler = server.WithClient(handler, os.DirFS(cfg.StaticDir))
	}
	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	var monitorDone chan struct{}
	if cfg.Notifications.Enabled {
		// Monitoring can poll faster without making each dashboard client
		// generate an independent collection. Failures retain a longer cooldown.
		source.Cooldown = cfg.Notifications.Interval
		db, err := storage.Open(cfg.Notifications.Database)
		if err != nil {
			log.Fatal("Could not open notification database")
		}
		defer db.Close()
		missing.DB = db
		monitorDone = make(chan struct{})
		go func() {
			defer close(monitorDone)
			notifications.Run(stop, db, client, cfg.Notifications, source.Get)
		}()
		log.Printf("Notification monitor enabled: interval=%s; saved baseline reused if present", cfg.Notifications.Interval)
	} else {
		log.Print("Notification monitor disabled: set NOTIFICATIONS_ENABLED=true and restart to enable")
	}
	go func() {
		log.Printf("reqarr listening on %s", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server error: %v", err)
			cancelStop()
		}
	}()

	<-stop.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	if monitorDone != nil {
		<-monitorDone
	}
}
