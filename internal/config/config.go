package config

import "os"

type Config struct {
	AppPort string

	PostgresDSN string

	MongoURI string
	MongoDB  string

	RedisAddr     string
	RedisPassword string

	Neo4jURI      string
	Neo4jUser     string
	Neo4jPassword string
}

func Load() Config {
	return Config{
		AppPort: getEnv("APP_PORT", "8080"),

		PostgresDSN: getEnv("POSTGRES_DSN", "postgres://app:app@localhost:5432/app?sslmode=disable"),

		MongoURI: getEnv("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:  getEnv("MONGO_DB", "app"),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		Neo4jURI:      getEnv("NEO4J_URI", "bolt://localhost:7687"),
		Neo4jUser:     getEnv("NEO4J_USER", "neo4j"),
		Neo4jPassword: getEnv("NEO4J_PASSWORD", "test12345"),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
