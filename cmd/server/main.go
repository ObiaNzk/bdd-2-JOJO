package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ObiaNzk/bdd-2-JOJO/cmd/server/internal/handler"
	"github.com/ObiaNzk/bdd-2-JOJO/internal/config"
	mongodbx "github.com/ObiaNzk/bdd-2-JOJO/internal/platform/mongodb"
	neo4jx "github.com/ObiaNzk/bdd-2-JOJO/internal/platform/neo4j"
	postgresx "github.com/ObiaNzk/bdd-2-JOJO/internal/platform/postgres"
	redisx "github.com/ObiaNzk/bdd-2-JOJO/internal/platform/redis"
	"github.com/ObiaNzk/bdd-2-JOJO/internal/repository"
	"github.com/ObiaNzk/bdd-2-JOJO/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()

	bootCtx, cancel := context.WithTimeout(rootCtx, 30*time.Second)
	defer cancel()

	pgPool, err := postgresx.New(bootCtx, cfg.PostgresDSN)
	if err != nil {
		return err
	}
	defer pgPool.Close()
	logger.Info("postgres connected")

	mongoClient, err := mongodbx.New(bootCtx, cfg.MongoURI)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = mongoClient.Disconnect(shutdownCtx)
	}()
	logger.Info("mongodb connected")

	redisClient, err := redisx.New(bootCtx, cfg.RedisAddr, cfg.RedisPassword)
	if err != nil {
		return err
	}
	defer redisClient.Close()
	logger.Info("redis connected")

	neoDriver, err := neo4jx.New(bootCtx, cfg.Neo4jURI, cfg.Neo4jUser, cfg.Neo4jPassword)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = neoDriver.Close(shutdownCtx)
	}()
	logger.Info("neo4j connected")

	repo := repository.New(pgPool, mongoClient.Database(cfg.MongoDB), redisClient, neoDriver)
	svc := service.New(repo)
	h := handler.New(svc)

	srv := &http.Server{
		Addr:              net.JoinHostPort("", cfg.AppPort),
		Handler:           h.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	select {
	case <-rootCtx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			return err
		}
		return nil
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return nil
}
