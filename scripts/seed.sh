#!/usr/bin/env bash
#
# Seeds three complete Olympic editions (Tokio 2020, París 2024, Los Ángeles 2028)
# across the four databases. Each edition has the same three disciplines, each
# realized by its own builder in the service:
#   - Natación 100m Libre  -> "race"           (carriles, parciales y tiempos en s)
#   - Salto con garrocha   -> "field_attempts" (alturas e intentos O/X/- en m)
#   - Fútbol 11            -> "tournament"     (cuadro octavos->final, goles y stats)
#
# Países: el script elige una cantidad aleatoria entre 16 y 20 de una lista
# maestra. El mínimo (16) es lo que necesita una ronda de octavos completa para
# que ningún equipo de fútbol sea filler. Para cada evento, un subconjunto
# aleatorio de los países entrados al juego forma los equipos (16 para fútbol;
# 4..8 para los individuales). Para que el caso 4 (atletas en múltiples
# disciplinas) tenga datos, en algunos países el nadador se agrega también al
# equipo de garrocha.
#
# Base entities go into Postgres via SQL; cada evento es "realizado" a través
# de la API HTTP (POST /events/{id}/realize), que invierte el resultado y
# replica medallas/resultados/records a Redis, Mongo y Neo4j.
#
# Idempotente: los 4 stores se vacían antes de sembrar.
#
# Usage:
#   ./scripts/seed.sh

set -euo pipefail

APP_URL="${APP_URL:-http://localhost:8080}"
NEO4J_PASSWORD="${NEO4J_PASSWORD:-test12345}"

cd "$(dirname "$0")/.."

echo "==> Waiting for the app at ${APP_URL}/healthz"
for _ in $(seq 1 30); do
  if curl -fsS "${APP_URL}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
curl -fsS "${APP_URL}/healthz" >/dev/null

echo "==> Flushing derived stores (Redis / Mongo / Neo4j)"
docker compose exec -T redis redis-cli FLUSHALL >/dev/null
docker compose exec -T mongo mongosh app --quiet --eval "db.event_results.drop()" >/dev/null
docker compose exec -T neo4j cypher-shell -u neo4j -p "${NEO4J_PASSWORD}" \
  "MATCH (n) DETACH DELETE n" >/dev/null

# Cantidad aleatoria de países entre 16 y 20. Los primeros tres del master
# (USA, Francia, Japón) son sedes y siempre quedan incluidos.
N=$((16 + RANDOM % 5))
echo "==> Seeding ${N} países (aleatorio entre 16 y 20) en 3 ediciones"

docker compose exec -T postgres psql -U app -d app -v ON_ERROR_STOP=1 -v "N=${N}" >/dev/null <<'SQL'
TRUNCATE medals, team_athletes, teams, events,
         game_countries, athletes, disciplines, sports, olympic_games, countries
RESTART IDENTITY CASCADE;

-- Master list de 20 países; tomamos los primeros :N. Las sedes (USA, Francia,
-- Japón) ocupan las tres primeras posiciones para que siempre estén presentes.
WITH master(ord, name) AS (
  VALUES
    (1,  'Estados Unidos'),
    (2,  'Francia'),
    (3,  'Japón'),
    (4,  'Australia'),
    (5,  'Suecia'),
    (6,  'Gran Bretaña'),
    (7,  'Brasil'),
    (8,  'España'),
    (9,  'Alemania'),
    (10, 'Italia'),
    (11, 'Países Bajos'),
    (12, 'Portugal'),
    (13, 'Croacia'),
    (14, 'Bélgica'),
    (15, 'México'),
    (16, 'Argentina'),
    (17, 'Canadá'),
    (18, 'Corea del Sur'),
    (19, 'Marruecos'),
    (20, 'Nigeria')
)
INSERT INTO countries (name)
SELECT name FROM master WHERE ord <= :N ORDER BY ord;

-- 3 deportes, 1 disciplina por deporte.
INSERT INTO sports (name) VALUES ('Natación'), ('Atletismo'), ('Fútbol');
INSERT INTO disciplines (sport_id, name) VALUES
  (1, '100m Libre'),
  (2, 'Salto con garrocha'),
  (3, 'Fútbol 11');

-- 3 juegos olímpicos; los hosts se resuelven por nombre.
INSERT INTO olympic_games (year, city, host_country_id) VALUES
  (2020, 'Tokio',       (SELECT id FROM countries WHERE name = 'Japón')),
  (2024, 'París',       (SELECT id FROM countries WHERE name = 'Francia')),
  (2028, 'Los Ángeles', (SELECT id FROM countries WHERE name = 'Estados Unidos'));

-- Todos los países participan en todos los juegos.
INSERT INTO game_countries (game_id, country_id)
SELECT g.id, c.id
FROM olympic_games g CROSS JOIN countries c
ORDER BY g.id, c.id;

-- 9 eventos: uno por (juego, disciplina).
INSERT INTO events (game_id, discipline_id, name, event_date)
SELECT g.id, d.id,
       'Final ' || d.name || ' ' || g.city,
       make_date(g.year, 8, 1)
FROM olympic_games g CROSS JOIN disciplines d
ORDER BY g.id, d.id;

-- Atletas: 1 nadador + 1 garrochista + 11 futbolistas por país.
INSERT INTO athletes (country_id, name)
SELECT c.id, 'Nadador ' || c.name FROM countries c ORDER BY c.id;

INSERT INTO athletes (country_id, name)
SELECT c.id, 'Garrochista ' || c.name FROM countries c ORDER BY c.id;

INSERT INTO athletes (country_id, name)
SELECT c.id, 'Futbolista ' || c.name || ' ' || g
FROM countries c CROSS JOIN generate_series(1, 11) g
ORDER BY c.id, g;

-- Equipos:
--   * Fútbol: 16 países (ronda de 16 sin filler).
--   * Individuales: 4..8 países random por evento (siempre >= 3).
INSERT INTO teams (game_country_id, event_id)
SELECT gc.id, e.id
FROM events e
JOIN disciplines d ON d.id = e.discipline_id
CROSS JOIN LATERAL (
  SELECT gc.id
  FROM game_countries gc
  WHERE gc.game_id = e.game_id
  ORDER BY random()
  LIMIT 16
) gc
WHERE d.name = 'Fútbol 11';

INSERT INTO teams (game_country_id, event_id)
SELECT gc.id, e.id
FROM events e
JOIN disciplines d ON d.id = e.discipline_id
CROSS JOIN LATERAL (
  SELECT gc.id
  FROM game_countries gc
  WHERE gc.game_id = e.game_id
  ORDER BY random()
  LIMIT 4 + floor(random() * 5)::int
) gc
WHERE d.name <> 'Fútbol 11';

-- Plantel de cada equipo (matching por prefijo del nombre del atleta).
INSERT INTO team_athletes (team_id, athlete_id)
SELECT t.id, a.id
FROM teams t
JOIN events e          ON e.id = t.event_id
JOIN disciplines d     ON d.id = e.discipline_id AND d.name = '100m Libre'
JOIN game_countries gc ON gc.id = t.game_country_id
JOIN athletes a        ON a.country_id = gc.country_id AND a.name LIKE 'Nadador %';

INSERT INTO team_athletes (team_id, athlete_id)
SELECT t.id, a.id
FROM teams t
JOIN events e          ON e.id = t.event_id
JOIN disciplines d     ON d.id = e.discipline_id AND d.name = 'Salto con garrocha'
JOIN game_countries gc ON gc.id = t.game_country_id
JOIN athletes a        ON a.country_id = gc.country_id AND a.name LIKE 'Garrochista %';

INSERT INTO team_athletes (team_id, athlete_id)
SELECT t.id, a.id
FROM teams t
JOIN events e          ON e.id = t.event_id
JOIN disciplines d     ON d.id = e.discipline_id AND d.name = 'Fútbol 11'
JOIN game_countries gc ON gc.id = t.game_country_id
JOIN athletes a        ON a.country_id = gc.country_id AND a.name LIKE 'Futbolista %';

-- Multi-disciplina (case 4): hasta 3 pares (juego, país) donde el país tiene
-- equipo en natación y en salto. Para esos, agregamos al nadador como atleta
-- extra del equipo de salto, así medalla en ambas disciplinas.
WITH swim AS (
  SELECT t.id AS team_id, gc.country_id, gc.game_id
  FROM teams t
  JOIN events e          ON e.id = t.event_id
  JOIN disciplines d     ON d.id = e.discipline_id AND d.name = '100m Libre'
  JOIN game_countries gc ON gc.id = t.game_country_id
),
vault AS (
  SELECT t.id AS team_id, gc.country_id, gc.game_id
  FROM teams t
  JOIN events e          ON e.id = t.event_id
  JOIN disciplines d     ON d.id = e.discipline_id AND d.name = 'Salto con garrocha'
  JOIN game_countries gc ON gc.id = t.game_country_id
),
overlap AS (
  SELECT s.country_id, v.team_id AS vault_team_id
  FROM swim s
  JOIN vault v ON v.game_id = s.game_id AND v.country_id = s.country_id
  ORDER BY random()
  LIMIT 3
)
INSERT INTO team_athletes (team_id, athlete_id)
SELECT o.vault_team_id, a.id
FROM overlap o
JOIN athletes a ON a.country_id = o.country_id AND a.name LIKE 'Nadador %';
SQL

api_post() {
  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "${APP_URL}$1" \
    -H 'Content-Type: application/json' -d "${2:-}")
  case "$code" in
    2*) printf '.' ;;
    *)  printf '\n[ERROR %s] POST %s\n' "$code" "$1" ;;
  esac
}

# Realizamos los 9 eventos (3 por edición).
echo "==> Realizing 9 events via the API (POST /events/{id}/realize)"
for ev in 1 2 3 4 5 6 7 8 9; do
  api_post "/events/${ev}/realize"
done
echo

echo "==> Neo4j graph summary"
docker compose exec -T neo4j cypher-shell -u neo4j -p "${NEO4J_PASSWORD}" --format plain \
  "MATCH (n) UNWIND labels(n) AS l RETURN l AS node, count(*) AS count ORDER BY l"
docker compose exec -T neo4j cypher-shell -u neo4j -p "${NEO4J_PASSWORD}" --format plain \
  "MATCH ()-[r]->() RETURN type(r) AS relationship, count(*) AS count ORDER BY type(r)"

echo "==> Done. 3 ediciones (Tokio 2020, París 2024, Los Ángeles 2028) x 3 disciplinas."
echo "    API:"
echo "      curl -s ${APP_URL}/games/latest/medals | jq"
echo "      curl -s ${APP_URL}/athletes/multi-discipline?min=2 | jq"
echo "      curl -s ${APP_URL}/records | jq"
echo "      curl -s ${APP_URL}/top-athletes?min=2 | jq                        # cross-games"
echo "      curl -s ${APP_URL}/countries/1/medals-by-discipline | jq          # cross-games"
echo "      curl -s '${APP_URL}/event-results?discipline=2' | jq              # salto con garrocha"
echo "      curl -s '${APP_URL}/event-results?discipline=3' | jq              # fútbol (cuadro completo)"
