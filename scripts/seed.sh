#!/usr/bin/env bash
#
# Seeds three complete Olympic editions (Tokio 2020, París 2024, Los Ángeles 2028)
# across the four databases. Each edition has the same three disciplines, each
# realized by its own builder in the service:
#   - Natación 100m Libre  -> "race"           (carriles, parciales y tiempos en s)
#   - Salto con garrocha   -> "field_attempts" (alturas e intentos O/X/- en m)
#   - Fútbol 11            -> "tournament"     (torneo encadenado por edición:
#                            semifinal -> final + tercer puesto; la final y el
#                            tercer puesto se CREAN al realizar la semifinal, que
#                            propaga ganadores/perdedores; medallas solo en esas
#                            dos rondas finales)
#
# Países: el script elige una cantidad aleatoria entre 16 y 20 de una lista
# maestra. Para cada evento individual, un subconjunto aleatorio de los países
# entrados al juego forma los equipos (4..8). El fútbol siembra 4 equipos en la
# semifinal de cada edición; la final y el tercer puesto se crean al realizar la
# semifinal. Para que el caso 4 (atletas en múltiples
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
docker compose exec -T mongo mongosh app --quiet --eval "db.event_results.drop(); db.world_records.drop()" >/dev/null
docker compose exec -T neo4j cypher-shell -u neo4j -p "${NEO4J_PASSWORD}" \
  "MATCH (n) DETACH DELETE n" >/dev/null

# Cantidad aleatoria de países entre 16 y 20. Los primeros tres del master
# (USA, Francia, Japón) son sedes y siempre quedan incluidos. El fútbol ya solo
# necesita 4 equipos para la semifinal; mantenemos el rango amplio para darle
# variedad de países a los eventos individuales.
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

-- Eventos individuales (natación, garrocha): un único "Final" por (juego,
-- disciplina), sin fase ni evento previo.
INSERT INTO events (game_id, discipline_id, name, event_date)
SELECT g.id, d.id,
       'Final ' || d.name || ' ' || g.city,
       make_date(g.year, 8, 1)
FROM olympic_games g CROSS JOIN disciplines d
WHERE d.name <> 'Fútbol 11'
ORDER BY g.id, d.id;

-- Fútbol: solo se siembra la semifinal de cada edición. La final y el tercer
-- puesto los crea el servicio al realizar la semifinal (con sus equipos ya
-- propagados), así que aquí no se insertan.
INSERT INTO events (game_id, discipline_id, name, event_date, phase)
SELECT g.id, d.id, 'Semifinal Fútbol ' || g.city, make_date(g.year, 8, 1), 'semifinal'
FROM olympic_games g CROSS JOIN disciplines d
WHERE d.name = 'Fútbol 11'
ORDER BY g.id;

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
--   * Fútbol: 4 países, solo en la semifinal (la final y el tercer puesto se
--     crean con sus equipos al realizarla).
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
  LIMIT 4
) gc
WHERE d.name = 'Fútbol 11' AND e.phase = 'semifinal';

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
JOIN athletes a        ON a.country_id = gc.country_id AND a.name LIKE 'Futbolista %'
WHERE e.phase = 'semifinal';

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

# El SQL de arriba cargó game_countries (y el resto de las entidades base) por
# acceso directo, después de que el app ya había arrancado y de haber flusheado
# Neo4j. Reiniciamos el app para que su SyncBaseEntities vuelva a espejar las
# entidades base al grafo —incluida Country-[:PARTICIPATES_IN]->OlympicGame— ahora
# que esos datos ya existen.
echo "==> Restarting app to mirror base entities (incl. country-in-game) to Neo4j"
docker compose restart app >/dev/null
for _ in $(seq 1 30); do
  if curl -fsS "${APP_URL}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
curl -fsS "${APP_URL}/healthz" >/dev/null

api_post() {
  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "${APP_URL}$1" \
    -H 'Content-Type: application/json' -d "${2:-}")
  case "$code" in
    2*) printf '.' ;;
    *)  printf '\n[ERROR %s] POST %s\n' "$code" "$1" ;;
  esac
}

# Realizamos en dos pasadas: primero los eventos sembrados (finales individuales
# + semifinales de fútbol). Realizar una semifinal crea su final y su tercer
# puesto con equipos, así que la segunda pasada toma esos eventos recién creados
# (los únicos que quedan sin realizar) y los realiza.
echo "==> Realizing events via the API (POST /events/{id}/realize)"

# scenarioMark scripts a deterministic world-record narrative across the three
# editions (realized in chronological order): Tokio sets the WR, París does NOT
# beat it, and Los Ángeles replaces Tokio's record. Returns the winning mark for
# an individual final given its discipline and edition year, or empty for events
# that should keep a random mark (e.g. fútbol). 100m Libre is lower-better;
# Salto con garrocha is higher-better.
scenarioMark() {
  local discipline="$1" year="$2"
  case "${discipline}|${year}" in
    "100m Libre|2020") echo "46.90" ;;  # WR
    "100m Libre|2024") echo "47.30" ;;  # no supera
    "100m Libre|2028") echo "46.50" ;;  # rompe el de Tokio
    "Salto con garrocha|2020") echo "6.00" ;;  # WR
    "Salto con garrocha|2024") echo "5.90" ;;  # no supera
    "Salto con garrocha|2028") echo "6.10" ;;  # rompe el de Tokio
    *) echo "" ;;
  esac
}

realize_pending() {
  local rows
  rows=$(docker compose exec -T postgres psql -U app -d app -tA -F'|' -c \
    "SELECT e.id, d.name, g.year
       FROM events e
       JOIN disciplines d   ON d.id = e.discipline_id
       JOIN olympic_games g ON g.id = e.game_id
      WHERE NOT e.realized
      ORDER BY (e.previous_event_id IS NOT NULL), e.game_id, e.id;")
  local id discipline year mark path
  while IFS='|' read -r id discipline year; do
    [ -z "${id}" ] && continue
    mark=$(scenarioMark "${discipline}" "${year}")
    path="/events/${id}/realize"
    [ -n "${mark}" ] && path="${path}?winnerMark=${mark}"
    api_post "${path}"
  done <<< "${rows}"
}
realize_pending   # finales individuales + semifinales de fútbol
realize_pending   # finales y terceros puestos creados al realizar las semifinales
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
echo "      curl -s '${APP_URL}/event-results?discipline=3' | jq              # fútbol (semifinal/final/tercer puesto)"
