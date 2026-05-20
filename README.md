# bdd-2-JOJO

TP juegos olímpicos, 8 profe así promociono.

## Estructura

```
cmd/server/                 binario principal (composition root)
  main.go                   http.Server + graceful shutdown + wiring
  internal/handler/         capa de handlers HTTP (chi)
internal/
  config/                   carga de variables de entorno
  platform/                 clientes de BD (postgres, mongodb, redis, neo4j)
  service/                  capa de lógica de negocio
  repository/               capa de acceso a datos
```

## Requisitos

- Go 1.26+
- Docker + Docker Compose

## Cómo correr

Con Docker (recomendado, levanta las 4 BDs + la app):

```
make up
curl localhost:8080/healthz
make logs
make down
```

Localmente (necesita las 4 BDs corriendo):

```
cp .env.example .env
make run
```
