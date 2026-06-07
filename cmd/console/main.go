// Command console is an interactive admin tool for the Olympic service. It lets
// you set up the base entities (countries, games, sports, disciplines,
// athletes, events, teams, rosters) directly in Postgres, and then "realize" an
// event — which generates fake results, medals and records and fans them out
// across Postgres, Redis, Mongo and Neo4j (the same orchestration the HTTP API
// uses).
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ObiaNzk/bdd-2-JOJO/internal/config"
	mongodbx "github.com/ObiaNzk/bdd-2-JOJO/internal/platform/mongodb"
	neo4jx "github.com/ObiaNzk/bdd-2-JOJO/internal/platform/neo4j"
	postgresx "github.com/ObiaNzk/bdd-2-JOJO/internal/platform/postgres"
	redisx "github.com/ObiaNzk/bdd-2-JOJO/internal/platform/redis"
	"github.com/ObiaNzk/bdd-2-JOJO/internal/repository"
	"github.com/ObiaNzk/bdd-2-JOJO/internal/service"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "console error:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	cfg := config.Load()

	bootCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	pgPool, err := postgresx.New(bootCtx, cfg.PostgresDSN)
	if err != nil {
		return err
	}
	defer pgPool.Close()

	mongoClient, err := mongodbx.New(bootCtx, cfg.MongoURI)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = mongoClient.Disconnect(shutdownCtx)
	}()

	redisClient, err := redisx.New(bootCtx, cfg.RedisAddr, cfg.RedisPassword)
	if err != nil {
		return err
	}
	defer redisClient.Close()

	neoDriver, err := neo4jx.New(bootCtx, cfg.Neo4jURI, cfg.Neo4jUser, cfg.Neo4jPassword)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = neoDriver.Close(shutdownCtx)
	}()

	sqlRepo := repository.NewPostgresRepository(pgPool)
	cacheRepo := repository.NewRedisRepository(redisClient)
	resultRepo := repository.NewMongoRepository(mongoClient.Database(cfg.MongoDB))
	graphRepo := repository.NewNeo4jRepository(neoDriver)

	if err := resultRepo.EnsureIndexes(bootCtx); err != nil {
		return err
	}
	if err := graphRepo.EnsureConstraints(bootCtx); err != nil {
		return err
	}

	countriesCreated, athletesCreated, err := sqlRepo.EnsureDefaults(bootCtx)
	if err != nil {
		return err
	}
	if countriesCreated+athletesCreated > 0 {
		fmt.Printf("datos por defecto cargados: %d países, %d deportistas\n", countriesCreated, athletesCreated)
	}

	svc := service.New(sqlRepo, cacheRepo, resultRepo, graphRepo)

	if err := svc.SyncBaseEntities(bootCtx); err != nil {
		return err
	}

	return newConsole(sqlRepo, svc, cacheRepo).Run(ctx)
}
