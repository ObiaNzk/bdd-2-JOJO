#!/usr/bin/env bash
#
# Seeds two Olympic editions (Tokio 2020, París 2024) across the four databases
# with three disciplines, each realized by its own builder in the service:
#   - Natación 100m Libre  -> "race"           (carriles, parciales y tiempos en s)
#   - Salto con garrocha   -> "field_attempts" (alturas e intentos O/X/- en m)
#   - Fútbol 11            -> "tournament"      (cuadro octavos->final, goles y stats)
# Base entities go into Postgres via SQL; then each event is "realized" through
# the HTTP API (POST /events/{id}/realize), so the service invents the result and
# fans medals/results/records out to Redis, Mongo and Neo4j.
#
# Idempotent: every store is flushed before seeding, so it is safe to re-run.
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

echo "==> Seeding base entities into Postgres"
docker compose exec -T postgres psql -U app -d app -v ON_ERROR_STOP=1 >/dev/null <<'SQL'
TRUNCATE medals, team_athletes, teams, events,
         game_countries, athletes, disciplines, sports, olympic_games, countries
RESTART IDENTITY CASCADE;

-- Countries (1..6)
INSERT INTO countries (name) VALUES
  ('Estados Unidos'),  -- 1
  ('Francia'),         -- 2
  ('Australia'),       -- 3
  ('Suecia'),          -- 4
  ('Japón'),           -- 5
  ('Gran Bretaña');    -- 6

-- Sports (1 Natación, 2 Atletismo, 3 Fútbol)
INSERT INTO sports (name) VALUES ('Natación'), ('Atletismo'), ('Fútbol');

-- Disciplines (1..3)
INSERT INTO disciplines (sport_id, name) VALUES
  (1,'100m Libre'),          -- 1 Natación
  (2,'Salto con garrocha'),  -- 2 Atletismo
  (3,'Fútbol 11');           -- 3 Fútbol

-- Olympic games (1 Tokio 2020 host JPN, 2 París 2024 host FRA)
INSERT INTO olympic_games (year, city, host_country_id) VALUES
  (2020,'Tokio',5),
  (2024,'París',2);

-- Participating countries per game (game_countries 1..12)
INSERT INTO game_countries (game_id, country_id) VALUES
  (1,1),(1,2),(1,3),(1,4),(1,5),(1,6),   -- 1..6  Tokio
  (2,1),(2,2),(2,3),(2,4),(2,5),(2,6);   -- 7..12 París

-- Events (1..5). e5 (Fútbol Tokio) queda SIN realizar para la demo de la consola.
INSERT INTO events (game_id, discipline_id, name, event_date) VALUES
  (2,1,'Final 100m Libre','2024-07-31'),         -- 1 París Natación
  (2,2,'Final Salto con garrocha','2024-08-05'), -- 2 París Salto
  (2,3,'Final Fútbol 11','2024-08-09'),          -- 3 París Fútbol
  (1,1,'Final 100m Libre','2020-07-29'),         -- 4 Tokio Natación
  (1,3,'Final Fútbol 11','2020-08-07');          -- 5 Tokio Fútbol (sin realizar)

-- Individual athletes (a1..a5). Kyle Chalmers compite en 100m y en garrocha:
-- es el atleta multi-disciplina para el caso 4.
INSERT INTO athletes (country_id, name) VALUES
  (1,'Caeleb Dressel'),    -- 1 USA  natación
  (2,'Maxime Grousset'),   -- 2 FRA  natación
  (3,'Kyle Chalmers'),     -- 3 AUS  natación + garrocha
  (4,'Armand Duplantis'),  -- 4 SWE  garrocha
  (1,'Sam Kendricks');     -- 5 USA  garrocha

-- Football rosters: 11 players per nation (FRA, GBR, USA, AUS).
INSERT INTO athletes (country_id, name) SELECT 2, 'Futbolista FRA ' || g FROM generate_series(1,11) g;
INSERT INTO athletes (country_id, name) SELECT 6, 'Futbolista GBR ' || g FROM generate_series(1,11) g;
INSERT INTO athletes (country_id, name) SELECT 1, 'Futbolista USA ' || g FROM generate_series(1,11) g;
INSERT INTO athletes (country_id, name) SELECT 3, 'Futbolista AUS ' || g FROM generate_series(1,11) g;

-- Teams (t1..t17).
INSERT INTO teams (game_country_id, event_id) VALUES
  -- e1 100m Libre París: USA, FRA, AUS
  (7,1),(8,1),(9,1),         -- t1,t2,t3
  -- e2 Salto París: SWE, USA, AUS
  (10,2),(7,2),(9,2),        -- t4,t5,t6
  -- e3 Fútbol París: FRA, GBR, USA, AUS
  (8,3),(12,3),(7,3),(9,3),  -- t7,t8,t9,t10
  -- e4 100m Libre Tokio: USA, FRA, AUS
  (1,4),(2,4),(3,4),         -- t11,t12,t13
  -- e5 Fútbol Tokio (sin realizar): FRA, GBR, USA, AUS
  (2,5),(6,5),(1,5),(3,5);   -- t14,t15,t16,t17

-- Individual rosters (one athlete per swimming/vault team).
INSERT INTO team_athletes (team_id, athlete_id) VALUES
  (1,1),(2,2),(3,3),     -- e1 100m: Dressel, Grousset, Chalmers
  (4,4),(5,5),(6,3),     -- e2 garrocha: Duplantis, Kendricks, Chalmers (multi-disciplina)
  (11,1),(12,2),(13,3);  -- e4 100m: Dressel, Grousset, Chalmers

-- Football rosters: 11 players per team (matched by country).
INSERT INTO team_athletes (team_id, athlete_id)
SELECT t.id, a.id
FROM teams t
JOIN events e          ON e.id = t.event_id AND e.discipline_id = 3
JOIN game_countries gc ON gc.id = t.game_country_id
JOIN athletes a        ON a.country_id = gc.country_id AND a.name LIKE 'Futbolista %';
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

# Realize every medal event (e5 stays unrealized for the console demo). Each call
# runs the discipline-specific builder that invents the result and fans it out.
echo "==> Realizing events via the API (POST /events/{id}/realize)"
for ev in 1 2 3 4; do
  api_post "/events/${ev}/realize"
done
echo

echo "==> Neo4j graph summary"
docker compose exec -T neo4j cypher-shell -u neo4j -p "${NEO4J_PASSWORD}" --format plain \
  "MATCH (n) UNWIND labels(n) AS l RETURN l AS node, count(*) AS count ORDER BY l"
docker compose exec -T neo4j cypher-shell -u neo4j -p "${NEO4J_PASSWORD}" --format plain \
  "MATCH ()-[r]->() RETURN type(r) AS relationship, count(*) AS count ORDER BY type(r)"

echo "==> Done. 3 disciplinas (100m Libre, Salto con garrocha, Fútbol 11) en 2 ediciones."
echo "    Demo consola: make console -> opción 10 -> realizar evento 5 (Final Fútbol 11 Tokio, sin realizar)."
echo "    API:"
echo "      curl -s ${APP_URL}/games/latest/medals | jq"
echo "      curl -s ${APP_URL}/athletes/multi-discipline?min=2 | jq"
echo "      curl -s ${APP_URL}/records | jq"
echo "      curl -s '${APP_URL}/event-results?discipline=2' | jq   # salto con garrocha"
echo "      curl -s '${APP_URL}/event-results?discipline=3' | jq   # fútbol (cuadro completo)"
