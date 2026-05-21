// Package service holds the business logic. It declares the interfaces it
// needs from each backend (consumer-side interfaces) and keeps Postgres as
// the source of truth, fanning every write out to the derived stores
// (Redis, Mongo, Neo4j).
package service

import (
	"context"
	"fmt"

	"github.com/ObiaNzk/bdd-2-JOJO/internal/model"
)

// SQLStore is the relational source of truth (Postgres).
type SQLStore interface {
	GetLatestOlympicGame(ctx context.Context) (*model.OlympicGame, error)
	GetEventByID(ctx context.Context, id int64) (*model.Event, error)
	GetTeamGraph(ctx context.Context, teamID int64) (*model.TeamGraph, error)
	ListTeamsByEvent(ctx context.Context, eventID int64) ([]int64, error)
	MarkEventRealized(ctx context.Context, eventID int64) error
	CreateMedal(ctx context.Context, m *model.Medal) error
	CountMedalsByCountryInGame(ctx context.Context, gameID int64) ([]model.MedalCount, error)
	ListEventsByPopularity(ctx context.Context, gameID int64, limit int) ([]model.EventPopularity, error)
	GetHostByGame(ctx context.Context, gameID int64) (*model.HostInfo, error)
	ListHosts(ctx context.Context) ([]model.HostInfo, error)
	ListCountries(ctx context.Context) ([]model.Country, error)
	ListOlympicGames(ctx context.Context) ([]model.OlympicGame, error)
	ListSports(ctx context.Context) ([]model.Sport, error)
	ListDisciplines(ctx context.Context) ([]model.Discipline, error)
	ListEvents(ctx context.Context) ([]model.Event, error)
	ListAthletesByCountry(ctx context.Context, countryID int64) ([]model.Athlete, error)
}

// MedalCache holds the Redis leaderboards and popularity counters.
type MedalCache interface {
	IncrCountryMedal(ctx context.Context, gameID int64, countryName string, medalType model.MedalType) error
	IncrAthleteMedal(ctx context.Context, gameID int64, athleteName string) error
	GetMedalRanking(ctx context.Context, gameID int64, limit int) ([]model.MedalCount, error)
	GetAthletesWithMinMedals(ctx context.Context, gameID int64, min int) ([]model.TopAthlete, error)
	GetAthleteMedalCount(ctx context.Context, gameID int64, athleteName string) (int, error)
	AddCountryToEvent(ctx context.Context, eventID, countryID int64) error
	GetEventPopularity(ctx context.Context, eventID int64) (int64, error)
}

// ResultStore holds the Mongo event results (type-specific event outcomes,
// from which olympic records are derived).
type ResultStore interface {
	RegisterEventResult(ctx context.Context, res *model.EventResult) error
	ListEventResults(ctx context.Context) ([]model.EventResult, error)
	ListEventResultsByDiscipline(ctx context.Context, disciplineID int64) ([]model.EventResult, error)
	ListRecordHolders(ctx context.Context) ([]model.RecordHolder, error)
	ListRecordHoldersByDiscipline(ctx context.Context, disciplineID int64) ([]model.RecordHolder, error)
	HasOlympicRecord(ctx context.Context, athleteID int64) (bool, error)
}

// GraphStore holds the Neo4j projection: the full team-centric graph of
// athletes, teams, events, disciplines, sports, games, countries and medals.
type GraphStore interface {
	SyncTeamGraph(ctx context.Context, tg *model.TeamGraph) error
	LinkTeamWonMedal(ctx context.Context, teamID, medalID int64, medalType model.MedalType) error
	ListAthletesWithMedalsInMultipleDisciplines(ctx context.Context, minDisciplines int) ([]model.AthleteDisciplines, error)
	CountMedalsByCountryAndDiscipline(ctx context.Context, countryID, gameID int64) ([]model.DisciplineMedalCount, error)
	UpsertCountry(ctx context.Context, c *model.Country) error
	UpsertOlympicGame(ctx context.Context, g *model.OlympicGame) error
	UpsertSport(ctx context.Context, s *model.Sport) error
	UpsertDiscipline(ctx context.Context, d *model.Discipline) error
	UpsertAthlete(ctx context.Context, a *model.Athlete) error
	UpsertEvent(ctx context.Context, e *model.Event) error
	LinkCountryToGame(ctx context.Context, gameID, countryID int64) error
}

type Service struct {
	sql     SQLStore
	cache   MedalCache
	results ResultStore
	graph   GraphStore
}

func New(sql SQLStore, cache MedalCache, results ResultStore, graph GraphStore) *Service {
	return &Service{sql: sql, cache: cache, results: results, graph: graph}
}

// --- Orchestrated writes ---

// AwardMedal persists a medal in Postgres and fans it out to Redis (country and
// athlete leaderboards) and Neo4j (the full team subgraph plus the won-medal
// relationship).
func (s *Service) AwardMedal(ctx context.Context, teamID int64, medalType model.MedalType) error {
	tg, err := s.sql.GetTeamGraph(ctx, teamID)
	if err != nil {
		return err
	}

	medal := &model.Medal{TeamID: teamID, Type: medalType}
	if err := s.sql.CreateMedal(ctx, medal); err != nil {
		return fmt.Errorf("create medal: %w", err)
	}

	if err := s.cache.IncrCountryMedal(ctx, tg.GameID, tg.CountryName, medalType); err != nil {
		return fmt.Errorf("incr country medal: %w", err)
	}
	for _, a := range tg.Athletes {
		if err := s.cache.IncrAthleteMedal(ctx, tg.GameID, a.Name); err != nil {
			return fmt.Errorf("incr athlete medal: %w", err)
		}
	}

	if err := s.graph.SyncTeamGraph(ctx, tg); err != nil {
		return fmt.Errorf("sync team graph: %w", err)
	}
	if err := s.graph.LinkTeamWonMedal(ctx, teamID, medal.ID, medalType); err != nil {
		return fmt.Errorf("link team-won medal: %w", err)
	}
	return nil
}

// RegisterParticipation mirrors a team's full subgraph into Neo4j (so teams that
// competed without medalling are still in the graph) and records the
// participating country in the Redis popularity counter (HyperLogLog). The
// finishing detail itself lives in the Mongo event-result document.
func (s *Service) RegisterParticipation(ctx context.Context, teamID int64) error {
	tg, err := s.sql.GetTeamGraph(ctx, teamID)
	if err != nil {
		return err
	}
	if err := s.graph.SyncTeamGraph(ctx, tg); err != nil {
		return fmt.Errorf("sync team graph: %w", err)
	}
	return s.cache.AddCountryToEvent(ctx, tg.EventID, tg.CountryID)
}

// RegisterEventResult stores a new event-result document in Mongo.
func (s *Service) RegisterEventResult(ctx context.Context, res *model.EventResult) error {
	return s.results.RegisterEventResult(ctx, res)
}

// --- Graph mirroring (base entities show up in Neo4j as they are created, not
// only when an event is realized) ---

func (s *Service) SyncCountry(ctx context.Context, c *model.Country) error {
	return s.graph.UpsertCountry(ctx, c)
}

func (s *Service) SyncOlympicGame(ctx context.Context, g *model.OlympicGame) error {
	return s.graph.UpsertOlympicGame(ctx, g)
}

func (s *Service) SyncSport(ctx context.Context, sp *model.Sport) error {
	return s.graph.UpsertSport(ctx, sp)
}

func (s *Service) SyncDiscipline(ctx context.Context, d *model.Discipline) error {
	return s.graph.UpsertDiscipline(ctx, d)
}

func (s *Service) SyncAthlete(ctx context.Context, a *model.Athlete) error {
	return s.graph.UpsertAthlete(ctx, a)
}

func (s *Service) SyncEvent(ctx context.Context, e *model.Event) error {
	return s.graph.UpsertEvent(ctx, e)
}

func (s *Service) SyncCountryInGame(ctx context.Context, gameID, countryID int64) error {
	return s.graph.LinkCountryToGame(ctx, gameID, countryID)
}

// SyncTeam mirrors a team's full neighbourhood (event, country, game, roster)
// into the graph, reusing the same projection the realize fan-out uses.
func (s *Service) SyncTeam(ctx context.Context, teamID int64) error {
	tg, err := s.sql.GetTeamGraph(ctx, teamID)
	if err != nil {
		return err
	}
	return s.graph.SyncTeamGraph(ctx, tg)
}

// SyncBaseEntities mirrors every base entity from Postgres into Neo4j. It is
// idempotent (all MERGE) and is run on startup so the graph reflects the data
// already in Postgres (e.g. the default countries and athletes).
func (s *Service) SyncBaseEntities(ctx context.Context) error {
	countries, err := s.sql.ListCountries(ctx)
	if err != nil {
		return err
	}
	for i := range countries {
		if err := s.graph.UpsertCountry(ctx, &countries[i]); err != nil {
			return err
		}
		athletes, err := s.sql.ListAthletesByCountry(ctx, countries[i].ID)
		if err != nil {
			return err
		}
		for j := range athletes {
			if err := s.graph.UpsertAthlete(ctx, &athletes[j]); err != nil {
				return err
			}
		}
	}

	games, err := s.sql.ListOlympicGames(ctx)
	if err != nil {
		return err
	}
	for i := range games {
		if err := s.graph.UpsertOlympicGame(ctx, &games[i]); err != nil {
			return err
		}
	}

	sports, err := s.sql.ListSports(ctx)
	if err != nil {
		return err
	}
	for i := range sports {
		if err := s.graph.UpsertSport(ctx, &sports[i]); err != nil {
			return err
		}
	}

	disciplines, err := s.sql.ListDisciplines(ctx)
	if err != nil {
		return err
	}
	for i := range disciplines {
		if err := s.graph.UpsertDiscipline(ctx, &disciplines[i]); err != nil {
			return err
		}
	}

	events, err := s.sql.ListEvents(ctx)
	if err != nil {
		return err
	}
	for i := range events {
		if err := s.graph.UpsertEvent(ctx, &events[i]); err != nil {
			return err
		}
	}
	return nil
}

// --- Use-case reads ---

// MedalRankingLatest resolves case 1 using the Redis leaderboard, which already
// stores country names as members.
func (s *Service) MedalRankingLatest(ctx context.Context, limit int) ([]model.MedalCount, error) {
	game, err := s.sql.GetLatestOlympicGame(ctx)
	if err != nil {
		return nil, err
	}
	return s.cache.GetMedalRanking(ctx, game.ID, limit)
}

// MedalRanking resolves case 1 for a specific game via the Redis leaderboard.
func (s *Service) MedalRanking(ctx context.Context, gameID int64, limit int) ([]model.MedalCount, error) {
	return s.cache.GetMedalRanking(ctx, gameID, limit)
}

// RecordHolders resolves case 2 (athletes that hold olympic records).
func (s *Service) RecordHolders(ctx context.Context) ([]model.RecordHolder, error) {
	return s.results.ListRecordHolders(ctx)
}

// RecordHoldersByDiscipline resolves case 2 filtered by discipline.
func (s *Service) RecordHoldersByDiscipline(ctx context.Context, disciplineID int64) ([]model.RecordHolder, error) {
	return s.results.ListRecordHoldersByDiscipline(ctx, disciplineID)
}

// EventResults returns the raw, type-specific event-result documents.
func (s *Service) EventResults(ctx context.Context) ([]model.EventResult, error) {
	return s.results.ListEventResults(ctx)
}

// EventResultsByDiscipline returns the raw event-result documents of a discipline.
func (s *Service) EventResultsByDiscipline(ctx context.Context, disciplineID int64) ([]model.EventResult, error) {
	return s.results.ListEventResultsByDiscipline(ctx, disciplineID)
}

// PopularEvents resolves case 3 with the exact Postgres count.
func (s *Service) PopularEvents(ctx context.Context, gameID int64, limit int) ([]model.EventPopularity, error) {
	return s.sql.ListEventsByPopularity(ctx, gameID, limit)
}

// EventPopularity resolves case 3 for a single event via the Redis HLL.
func (s *Service) EventPopularity(ctx context.Context, eventID int64) (int64, error) {
	return s.cache.GetEventPopularity(ctx, eventID)
}

// AthletesInMultipleDisciplines resolves case 4 via Neo4j.
func (s *Service) AthletesInMultipleDisciplines(ctx context.Context, minDisciplines int) ([]model.AthleteDisciplines, error) {
	return s.graph.ListAthletesWithMedalsInMultipleDisciplines(ctx, minDisciplines)
}

// HostByGame resolves case 5 for a single edition.
func (s *Service) HostByGame(ctx context.Context, gameID int64) (*model.HostInfo, error) {
	return s.sql.GetHostByGame(ctx, gameID)
}

// Hosts resolves case 5 for every edition.
func (s *Service) Hosts(ctx context.Context) ([]model.HostInfo, error) {
	return s.sql.ListHosts(ctx)
}

// MedalsByCountryAndDiscipline resolves case 6 over the Neo4j graph.
func (s *Service) MedalsByCountryAndDiscipline(ctx context.Context, countryID, gameID int64) ([]model.DisciplineMedalCount, error) {
	return s.graph.CountMedalsByCountryAndDiscipline(ctx, countryID, gameID)
}

// TopAthletes resolves case 7: athletes with at least minMedals OR a current
// olympic record, merging the Redis leaderboard with the Mongo records.
func (s *Service) TopAthletes(ctx context.Context, gameID int64, minMedals int) ([]model.TopAthlete, error) {
	byName := map[string]*model.TopAthlete{}

	withMedals, err := s.cache.GetAthletesWithMinMedals(ctx, gameID, minMedals)
	if err != nil {
		return nil, err
	}
	for i := range withMedals {
		a := withMedals[i]
		byName[a.AthleteName] = &a
	}

	recordHolders, err := s.results.ListRecordHolders(ctx)
	if err != nil {
		return nil, err
	}
	for _, rec := range recordHolders {
		entry, ok := byName[rec.AthleteName]
		if !ok {
			count, _ := s.cache.GetAthleteMedalCount(ctx, gameID, rec.AthleteName)
			entry = &model.TopAthlete{AthleteName: rec.AthleteName, TotalMedals: count}
			byName[rec.AthleteName] = entry
		}
		entry.HasRecord = true
	}

	out := make([]model.TopAthlete, 0, len(byName))
	for _, a := range byName {
		out = append(out, *a)
	}
	return out, nil
}
