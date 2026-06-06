-- Olympic Games relational schema (Postgres = source of truth).
-- Loaded automatically by the postgres container on first start.

CREATE TABLE IF NOT EXISTS countries (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS olympic_games (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    year            INT  NOT NULL,
    city            TEXT NOT NULL,
    host_country_id BIGINT NOT NULL REFERENCES countries(id)
);

CREATE TABLE IF NOT EXISTS sports (
    id   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS disciplines (
    id       BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sport_id BIGINT NOT NULL REFERENCES sports(id),
    name     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS athletes (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    country_id BIGINT NOT NULL REFERENCES countries(id),
    name       TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS game_countries (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    game_id    BIGINT NOT NULL REFERENCES olympic_games(id),
    country_id BIGINT NOT NULL REFERENCES countries(id),
    UNIQUE (game_id, country_id)
);

CREATE TABLE IF NOT EXISTS events (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    game_id       BIGINT NOT NULL REFERENCES olympic_games(id),
    discipline_id BIGINT NOT NULL REFERENCES disciplines(id),
    name          TEXT NOT NULL,
    event_date    DATE NOT NULL DEFAULT CURRENT_DATE,
    -- Tournament events split into chained rounds: phase names the round
    -- ('semifinal', 'final', 'tercer_puesto') and previous_event_id points back
    -- to the round it advances from. Non-tournament events leave both NULL.
    phase             TEXT,
    previous_event_id BIGINT REFERENCES events(id),
    realized          BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS teams (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    game_country_id BIGINT NOT NULL REFERENCES game_countries(id),
    event_id        BIGINT NOT NULL REFERENCES events(id)
);

CREATE TABLE IF NOT EXISTS team_athletes (
    team_id    BIGINT NOT NULL REFERENCES teams(id),
    athlete_id BIGINT NOT NULL REFERENCES athletes(id),
    PRIMARY KEY (team_id, athlete_id)
);

CREATE TABLE IF NOT EXISTS medals (
    id      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    team_id BIGINT NOT NULL REFERENCES teams(id),
    type    TEXT NOT NULL CHECK (type IN ('gold', 'silver', 'bronze'))
);

CREATE INDEX IF NOT EXISTS idx_disciplines_sport     ON disciplines(sport_id);
CREATE INDEX IF NOT EXISTS idx_athletes_country       ON athletes(country_id);
CREATE INDEX IF NOT EXISTS idx_events_game            ON events(game_id);
CREATE INDEX IF NOT EXISTS idx_events_discipline      ON events(discipline_id);
CREATE INDEX IF NOT EXISTS idx_events_previous         ON events(previous_event_id);
CREATE INDEX IF NOT EXISTS idx_teams_event            ON teams(event_id);
CREATE INDEX IF NOT EXISTS idx_teams_game_country     ON teams(game_country_id);
CREATE INDEX IF NOT EXISTS idx_medals_team            ON medals(team_id);
