package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ObiaNzk/bdd-2-JOJO/internal/model"
)

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// --- Country ---

func (r *PostgresRepository) CreateCountry(ctx context.Context, c *model.Country) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO countries (name) VALUES ($1) RETURNING id`,
		c.Name,
	).Scan(&c.ID)
}

func (r *PostgresRepository) GetCountryByID(ctx context.Context, id int64) (*model.Country, error) {
	var c model.Country
	err := r.db.QueryRow(ctx,
		`SELECT id, name FROM countries WHERE id = $1`, id,
	).Scan(&c.ID, &c.Name)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *PostgresRepository) ListCountries(ctx context.Context) ([]model.Country, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name FROM countries ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Country
	for rows.Next() {
		var c model.Country
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// --- OlympicGame ---

func (r *PostgresRepository) CreateOlympicGame(ctx context.Context, g *model.OlympicGame) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO olympic_games (year, city, host_country_id)
		 VALUES ($1, $2, $3) RETURNING id`,
		g.Year, g.City, g.HostCountryID,
	).Scan(&g.ID)
}

func (r *PostgresRepository) GetOlympicGameByID(ctx context.Context, id int64) (*model.OlympicGame, error) {
	var g model.OlympicGame
	err := r.db.QueryRow(ctx,
		`SELECT id, year, city, host_country_id FROM olympic_games WHERE id = $1`, id,
	).Scan(&g.ID, &g.Year, &g.City, &g.HostCountryID)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (r *PostgresRepository) GetLatestOlympicGame(ctx context.Context) (*model.OlympicGame, error) {
	var g model.OlympicGame
	err := r.db.QueryRow(ctx,
		`SELECT id, year, city, host_country_id FROM olympic_games ORDER BY year DESC LIMIT 1`,
	).Scan(&g.ID, &g.Year, &g.City, &g.HostCountryID)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// --- Sport / Discipline ---

func (r *PostgresRepository) CreateSport(ctx context.Context, s *model.Sport) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO sports (name) VALUES ($1) RETURNING id`, s.Name,
	).Scan(&s.ID)
}

func (r *PostgresRepository) CreateDiscipline(ctx context.Context, d *model.Discipline) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO disciplines (sport_id, name) VALUES ($1, $2) RETURNING id`,
		d.SportID, d.Name,
	).Scan(&d.ID)
}

func (r *PostgresRepository) ListDisciplinesBySport(ctx context.Context, sportID int64) ([]model.Discipline, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, sport_id, name FROM disciplines WHERE sport_id = $1 ORDER BY name`, sportID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Discipline
	for rows.Next() {
		var d model.Discipline
		if err := rows.Scan(&d.ID, &d.SportID, &d.Name); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// --- Athlete ---

func (r *PostgresRepository) CreateAthlete(ctx context.Context, a *model.Athlete) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO athletes (country_id, name) VALUES ($1, $2) RETURNING id`,
		a.CountryID, a.Name,
	).Scan(&a.ID)
}

func (r *PostgresRepository) GetAthleteByID(ctx context.Context, id int64) (*model.Athlete, error) {
	var a model.Athlete
	err := r.db.QueryRow(ctx,
		`SELECT id, country_id, name FROM athletes WHERE id = $1`, id,
	).Scan(&a.ID, &a.CountryID, &a.Name)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *PostgresRepository) ListAthletesByCountry(ctx context.Context, countryID int64) ([]model.Athlete, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, country_id, name FROM athletes WHERE country_id = $1 ORDER BY name`, countryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Athlete
	for rows.Next() {
		var a model.Athlete
		if err := rows.Scan(&a.ID, &a.CountryID, &a.Name); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// --- Event ---

func (r *PostgresRepository) CreateEvent(ctx context.Context, e *model.Event) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO events (game_id, discipline_id, name, event_date)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		e.GameID, e.DisciplineID, e.Name, e.Date,
	).Scan(&e.ID)
}

func (r *PostgresRepository) GetEventByID(ctx context.Context, id int64) (*model.Event, error) {
	var e model.Event
	err := r.db.QueryRow(ctx,
		`SELECT id, game_id, discipline_id, name, event_date, realized FROM events WHERE id = $1`, id,
	).Scan(&e.ID, &e.GameID, &e.DisciplineID, &e.Name, &e.Date, &e.Realized)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// MarkEventRealized flags an event as realized so it cannot be realized again.
func (r *PostgresRepository) MarkEventRealized(ctx context.Context, eventID int64) error {
	_, err := r.db.Exec(ctx, `UPDATE events SET realized = TRUE WHERE id = $1`, eventID)
	return err
}

func (r *PostgresRepository) ListEventsByGame(ctx context.Context, gameID int64) ([]model.Event, error) {
	return r.listEvents(ctx, `WHERE game_id = $1`, gameID)
}

func (r *PostgresRepository) ListEventsByDiscipline(ctx context.Context, disciplineID int64) ([]model.Event, error) {
	return r.listEvents(ctx, `WHERE discipline_id = $1`, disciplineID)
}

func (r *PostgresRepository) listEvents(ctx context.Context, where string, arg int64) ([]model.Event, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, game_id, discipline_id, name, event_date, realized FROM events `+where+` ORDER BY id`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Event
	for rows.Next() {
		var e model.Event
		if err := rows.Scan(&e.ID, &e.GameID, &e.DisciplineID, &e.Name, &e.Date, &e.Realized); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- GameCountry ---

func (r *PostgresRepository) RegisterCountryInGame(ctx context.Context, gc *model.GameCountry) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO game_countries (game_id, country_id) VALUES ($1, $2) RETURNING id`,
		gc.GameID, gc.CountryID,
	).Scan(&gc.ID)
}

// --- Team / TeamAthlete ---

func (r *PostgresRepository) CreateTeam(ctx context.Context, t *model.Team) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO teams (game_country_id, event_id) VALUES ($1, $2) RETURNING id`,
		t.GameCountryID, t.EventID,
	).Scan(&t.ID)
}

func (r *PostgresRepository) AddAthleteToTeam(ctx context.Context, teamID, athleteID int64) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO team_athletes (team_id, athlete_id) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`, teamID, athleteID)
	return err
}

func (r *PostgresRepository) ListAthletesByTeam(ctx context.Context, teamID int64) ([]model.Athlete, error) {
	rows, err := r.db.Query(ctx,
		`SELECT a.id, a.country_id, a.name
		 FROM athletes a
		 JOIN team_athletes ta ON ta.athlete_id = a.id
		 WHERE ta.team_id = $1`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Athlete
	for rows.Next() {
		var a model.Athlete
		if err := rows.Scan(&a.ID, &a.CountryID, &a.Name); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// --- Medal ---

func (r *PostgresRepository) CreateMedal(ctx context.Context, m *model.Medal) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO medals (team_id, type) VALUES ($1, $2) RETURNING id`,
		m.TeamID, string(m.Type),
	).Scan(&m.ID)
}

// GetTeamGraph returns the full denormalized neighbourhood of a team — event,
// discipline, sport, game, country and the athlete roster (each with its home
// country) — so the service can mirror the whole relational structure out to
// the derived stores (Redis, Neo4j).
func (r *PostgresRepository) GetTeamGraph(ctx context.Context, teamID int64) (*model.TeamGraph, error) {
	tg := model.TeamGraph{TeamID: teamID}
	err := r.db.QueryRow(ctx,
		`SELECT e.id, e.name,
		        d.id, d.name,
		        s.id, s.name,
		        g.id, g.year, g.city,
		        c.id, c.name
		 FROM teams t
		 JOIN game_countries gc ON gc.id = t.game_country_id
		 JOIN countries c       ON c.id = gc.country_id
		 JOIN olympic_games g   ON g.id = gc.game_id
		 JOIN events e          ON e.id = t.event_id
		 JOIN disciplines d     ON d.id = e.discipline_id
		 JOIN sports s          ON s.id = d.sport_id
		 WHERE t.id = $1`, teamID,
	).Scan(
		&tg.EventID, &tg.EventName,
		&tg.DisciplineID, &tg.DisciplineName,
		&tg.SportID, &tg.SportName,
		&tg.GameID, &tg.GameYear, &tg.GameCity,
		&tg.CountryID, &tg.CountryName,
	)
	if err != nil {
		return nil, fmt.Errorf("team graph: %w", err)
	}

	rows, err := r.db.Query(ctx,
		`SELECT a.id, a.name, c.id, c.name
		 FROM team_athletes ta
		 JOIN athletes a  ON a.id = ta.athlete_id
		 JOIN countries c ON c.id = a.country_id
		 WHERE ta.team_id = $1`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a model.GraphAthlete
		if err := rows.Scan(&a.ID, &a.Name, &a.CountryID, &a.CountryName); err != nil {
			return nil, err
		}
		tg.Athletes = append(tg.Athletes, a)
	}
	return &tg, rows.Err()
}

// --- Use-case queries ---

// CountMedalsByCountryInGame is the exact (Postgres) fallback for case 1.
func (r *PostgresRepository) CountMedalsByCountryInGame(ctx context.Context, gameID int64) ([]model.MedalCount, error) {
	rows, err := r.db.Query(ctx,
		`SELECT c.id, c.name,
		        COUNT(*) FILTER (WHERE m.type = 'gold')   AS gold,
		        COUNT(*) FILTER (WHERE m.type = 'silver') AS silver,
		        COUNT(*) FILTER (WHERE m.type = 'bronze') AS bronze,
		        COUNT(*)                                  AS total
		 FROM medals m
		 JOIN teams t          ON t.id = m.team_id
		 JOIN game_countries gc ON gc.id = t.game_country_id
		 JOIN countries c       ON c.id = gc.country_id
		 WHERE gc.game_id = $1
		 GROUP BY c.id, c.name
		 ORDER BY gold DESC, silver DESC, bronze DESC`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.MedalCount
	for rows.Next() {
		var mc model.MedalCount
		if err := rows.Scan(&mc.CountryID, &mc.CountryName, &mc.Gold, &mc.Silver, &mc.Bronze, &mc.Total); err != nil {
			return nil, err
		}
		out = append(out, mc)
	}
	return out, rows.Err()
}

// ListEventsByPopularity is the exact (Postgres) fallback for case 3.
func (r *PostgresRepository) ListEventsByPopularity(ctx context.Context, gameID int64, limit int) ([]model.EventPopularity, error) {
	rows, err := r.db.Query(ctx,
		`SELECT e.id, e.name, COUNT(DISTINCT gc.country_id) AS countries
		 FROM events e
		 JOIN teams t           ON t.event_id = e.id
		 JOIN game_countries gc ON gc.id = t.game_country_id
		 WHERE e.game_id = $1
		 GROUP BY e.id, e.name
		 ORDER BY countries DESC
		 LIMIT $2`, gameID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.EventPopularity
	for rows.Next() {
		var ep model.EventPopularity
		if err := rows.Scan(&ep.EventID, &ep.EventName, &ep.CountriesCount); err != nil {
			return nil, err
		}
		out = append(out, ep)
	}
	return out, rows.Err()
}

// GetHostByGame resolves case 5 for a single edition.
func (r *PostgresRepository) GetHostByGame(ctx context.Context, gameID int64) (*model.HostInfo, error) {
	var h model.HostInfo
	err := r.db.QueryRow(ctx,
		`SELECT g.id, g.year, g.city, c.id, c.name
		 FROM olympic_games g
		 JOIN countries c ON c.id = g.host_country_id
		 WHERE g.id = $1`, gameID,
	).Scan(&h.GameID, &h.Year, &h.City, &h.CountryID, &h.CountryName)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// ListHosts resolves case 5 for every edition.
func (r *PostgresRepository) ListHosts(ctx context.Context) ([]model.HostInfo, error) {
	rows, err := r.db.Query(ctx,
		`SELECT g.id, g.year, g.city, c.id, c.name
		 FROM olympic_games g
		 JOIN countries c ON c.id = g.host_country_id
		 ORDER BY g.year`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.HostInfo
	for rows.Next() {
		var h model.HostInfo
		if err := rows.Scan(&h.GameID, &h.Year, &h.City, &h.CountryID, &h.CountryName); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// --- Listings & lookups (console / admin support) ---

func (r *PostgresRepository) ListOlympicGames(ctx context.Context) ([]model.OlympicGame, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, year, city, host_country_id FROM olympic_games ORDER BY year`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.OlympicGame
	for rows.Next() {
		var g model.OlympicGame
		if err := rows.Scan(&g.ID, &g.Year, &g.City, &g.HostCountryID); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListSports(ctx context.Context) ([]model.Sport, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name FROM sports ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Sport
	for rows.Next() {
		var s model.Sport
		if err := rows.Scan(&s.ID, &s.Name); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListDisciplines(ctx context.Context) ([]model.Discipline, error) {
	rows, err := r.db.Query(ctx, `SELECT id, sport_id, name FROM disciplines ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Discipline
	for rows.Next() {
		var d model.Discipline
		if err := rows.Scan(&d.ID, &d.SportID, &d.Name); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListEvents(ctx context.Context) ([]model.Event, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, game_id, discipline_id, name, event_date, realized FROM events ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Event
	for rows.Next() {
		var e model.Event
		if err := rows.Scan(&e.ID, &e.GameID, &e.DisciplineID, &e.Name, &e.Date, &e.Realized); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListTeamsByEvent(ctx context.Context, eventID int64) ([]int64, error) {
	rows, err := r.db.Query(ctx, `SELECT id FROM teams WHERE event_id = $1 ORDER BY id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ListTeamsByEventWithCountry returns the teams entered in an event along with
// the country each one represents, for the console team picker.
func (r *PostgresRepository) ListTeamsByEventWithCountry(ctx context.Context, eventID int64) ([]model.TeamListing, error) {
	rows, err := r.db.Query(ctx,
		`SELECT t.id, c.id, c.name
		 FROM teams t
		 JOIN game_countries gc ON gc.id = t.game_country_id
		 JOIN countries c       ON c.id = gc.country_id
		 WHERE t.event_id = $1
		 ORDER BY t.id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.TeamListing
	for rows.Next() {
		var t model.TeamListing
		if err := rows.Scan(&t.TeamID, &t.CountryID, &t.CountryName); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetGameCountryID resolves the participation row tying a country to a game, so
// the console can create a team from an (event, country) pair.
func (r *PostgresRepository) GetGameCountryID(ctx context.Context, gameID, countryID int64) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx,
		`SELECT id FROM game_countries WHERE game_id = $1 AND country_id = $2`,
		gameID, countryID,
	).Scan(&id)
	return id, err
}
