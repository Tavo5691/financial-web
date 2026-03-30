package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"financial-web/internal/db"
	"financial-web/internal/web"

	// Register all parsers.
	_ "financial-web/internal/parser/bbva_visa"
	_ "financial-web/internal/parser/galicia_mastercard"
	_ "financial-web/internal/parser/galicia_visa"
	_ "financial-web/internal/parser/uala_mastercard"
)

func main() {
	dbPath := flag.String("db", "./data/financial.db", "path to SQLite database")
	port := flag.String("port", "8080", "HTTP listen port")
	inbox := flag.String("inbox", "./inbox", "directory to watch for new PDF statements")
	processed := flag.String("processed", "./processed", "directory to move PDFs after successful processing")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	database, err := db.Open(*dbPath)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	logger.Info("database ready", "path", *dbPath)

	server, err := web.New(database.DB, logger, *inbox, *processed)
	if err != nil {
		logger.Error("init server", "error", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	server.Watcher.Start(ctx)
	logger.Info("inbox watcher started", "dir", *inbox)

	httpServer := &http.Server{
		Addr:         ":" + *port,
		Handler:      server.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		<-ctx.Done()
		logger.Info("shutting down...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		httpServer.Shutdown(shutdownCtx)
	}()

	logger.Info("server ready", "addr", "http://localhost:"+*port)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
