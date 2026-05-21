# bdd-2-JOJO

Sistema de gestión de datos de los Juegos Olímpicos sobre 4 bases:
Postgres (fuente de verdad), Redis, MongoDB y Neo4j.

## Requisitos

- Docker + Docker Compose
- Go 1.26+ (solo si querés correr la app fuera de Docker)

## Puesta en marcha

Todo se maneja con el `Makefile`:

```bash
make install   # baja las imágenes de Docker (4 bases + visores) y construye la del app
make up        # levanta las 4 bases + app + visores web, y abre los visores en el navegador
```

La app queda en http://localhost:8080 (`GET /healthz` responde `{"status":"ok"}`).

Apagar y limpiar (borra los volúmenes/datos):

```bash
make down
```

Otros comandos útiles:

```bash
make console   # consola interactiva de administración (carga/consultas)
make logs      # seguir los logs del contenedor de la app
make run       # correr la app localmente (necesita las bases arriba)
```

### Alternativa: app local contra bases dockerizadas

```bash
docker compose up -d postgres mongo redis neo4j
make run
```

La config por defecto apunta a `localhost`, y los puertos están publicados.

## Datos

**No hace falta correr ningún seed.** Al iniciar, la app y la consola ya cargan
datos por defecto (5 países —Argentina incluida— con deportistas) y los reflejan
en Neo4j. Desde la consola (`make console`) podés crear todas las entidades,
generar equipos para un evento (opción 13) y realizar eventos (opción 10) — eso
es lo que llena Redis, MongoDB y Neo4j.

Opcional: si querés un dataset ya armado y realizado (Tokio 2020 + París 2024,
las 3 disciplinas) sin cargarlo a mano, con el stack arriba podés correr el
script de ejemplo (idempotente):

```bash
./scripts/seed.sh   # opcional
```

## Visores web

`make up` ya los levanta y los abre en el navegador. Si querés manejarlos
aparte (sin `make`), corren bajo el profile `tools`:

```bash
docker compose --profile tools up -d      # levantar
docker compose --profile tools down       # apagar (sin tocar las bases)
```

| Base       | Visor          | URL                     |
|------------|----------------|-------------------------|
| Postgres   | Adminer         | http://localhost:8081   |
| MongoDB    | mongo-express   | http://localhost:8082   |
| Redis      | Redis Commander | http://localhost:8083   |
| Neo4j      | Neo4j Browser   | http://localhost:7474   |

## Credenciales

### Conexión desde tu máquina (CLI / app local)

| Servicio | Host             | Puerto | Usuario | Password    | Base |
|----------|------------------|--------|---------|-------------|------|
| Postgres | localhost        | 5432   | `app`   | `app`       | `app` |
| MongoDB  | localhost        | 27017  | —       | —           | `app` |
| Redis    | localhost        | 6379   | —       | —           | —    |
| Neo4j    | localhost (7687) | 7687   | `neo4j` | `test12345` | —    |
| App HTTP | localhost        | 8080   | —       | —           | —    |

### Acceso desde cada visor web

Los visores corren dentro de la red de Docker, así que el **host** es el
nombre del servicio (no `localhost`):

- **Adminer** (http://localhost:8081): el formulario ya viene con
  System `PostgreSQL` y Server `postgres`. Completá User `app`, Password
  `app`, Database `app`.
- **mongo-express** (http://localhost:8082): sin login, ya conectado a
  MongoDB. La colección de resultados de eventos es `app` → `event_results`.
- **Redis Commander** (http://localhost:8083): ya conectado a la base
  (`local:redis:6379`). Tiene árbol de keys con búsqueda y una consola
  para correr comandos.
- **Neo4j Browser** (http://localhost:7474): Connect URL
  `bolt://localhost:7687`, usuario `neo4j`, password `test12345`.
