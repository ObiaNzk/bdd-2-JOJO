package service

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/ObiaNzk/bdd-2-JOJO/internal/model"
)

// A tournament is modelled as three chained events sharing a discipline:
//
//	semifinal (4 teams) --> final         (2 teams: the two winners)  -> gold + silver
//	                    \-> tercer_puesto (2 teams: the two losers)   -> bronze
//
// Only the semifinal is laid down up front (and is the only round given teams by
// hand). Realising it *creates* the final and the bronze match and advances the
// two winners and two losers into them (real propagation): the later rounds do
// not exist until the round that feeds them has run. Each event is one round and
// writes its own Mongo document; only the deciding rounds award medals.

// footballCompetitor is one team in a match, carrying the TeamGraph of a real
// entered team.
type footballCompetitor struct {
	graph *model.TeamGraph
}

func (c *footballCompetitor) teamID() int64    { return c.graph.TeamID }
func (c *footballCompetitor) countryID() int64 { return c.graph.CountryID }
func (c *footballCompetitor) name() string     { return c.graph.CountryName }

// realizeTournament runs one round of a tournament, branching on the event's
// phase. graphs is the (already shuffled) set of teams entered in this round.
func (s *Service) realizeTournament(ctx context.Context, event *model.Event, graphs []*model.TeamGraph, summary *model.RealizeSummary) (*model.RealizeSummary, error) {
	switch event.Phase {
	case "semifinal":
		return s.realizeSemifinal(ctx, event, graphs, summary)
	case "final":
		return s.realizeTournamentFinal(ctx, event, graphs, summary)
	case "tercer_puesto":
		return s.realizeThirdPlace(ctx, event, graphs, summary)
	default:
		return nil, fmt.Errorf("tournament event %d needs a phase (semifinal/final/tercer_puesto), got %q", event.ID, event.Phase)
	}
}

// realizeSemifinal plays the two semifinals, creates the final and the bronze
// match, advances the two winners to the final and the two losers to the bronze
// match, and writes one Mongo document. It awards no medals.
func (s *Service) realizeSemifinal(ctx context.Context, event *model.Event, graphs []*model.TeamGraph, summary *model.RealizeSummary) (*model.RealizeSummary, error) {
	if len(graphs) != 4 {
		return nil, fmt.Errorf("semifinal event %d requires exactly 4 teams, got %d", event.ID, len(graphs))
	}
	comps := competitors(graphs)

	// The first competitor of each pair wins (the field is already shuffled).
	matches := []map[string]any{
		buildMatch(comps[0], comps[1], "semifinal"),
		buildMatch(comps[2], comps[3], "semifinal"),
	}
	winners := []*model.TeamGraph{graphs[0], graphs[2]}
	losers := []*model.TeamGraph{graphs[1], graphs[3]}

	for _, g := range graphs {
		if err := s.RegisterParticipation(ctx, g.TeamID); err != nil {
			return nil, fmt.Errorf("register participation (team %d): %w", g.TeamID, err)
		}
	}

	// Create the next rounds now that the semifinal has run: the final and the
	// bronze match did not exist until now, since their teams were unknown.
	finalEvent, thirdEvent, err := s.createNextRounds(ctx, event)
	if err != nil {
		return nil, err
	}

	// Real propagation: the winners enter the final, the losers the bronze match.
	for _, w := range winners {
		if err := s.advanceTeam(ctx, finalEvent.ID, w); err != nil {
			return nil, fmt.Errorf("advance winner to final: %w", err)
		}
	}
	for _, l := range losers {
		if err := s.advanceTeam(ctx, thirdEvent.ID, l); err != nil {
			return nil, fmt.Errorf("advance loser to bronze match: %w", err)
		}
	}

	if err := s.registerTournamentRound(ctx, event, graphs[0], "semifinal", matches); err != nil {
		return nil, err
	}

	summary.Participants = len(graphs)
	summary.Records = 0
	return summary, nil
}

// realizeTournamentFinal plays the final: the winner takes gold, the loser silver.
func (s *Service) realizeTournamentFinal(ctx context.Context, event *model.Event, graphs []*model.TeamGraph, summary *model.RealizeSummary) (*model.RealizeSummary, error) {
	if len(graphs) != 2 {
		return nil, fmt.Errorf("final event %d requires exactly 2 teams, got %d", event.ID, len(graphs))
	}
	comps := competitors(graphs)
	match := buildMatch(comps[0], comps[1], "final") // comps[0] wins

	for _, g := range graphs {
		if err := s.RegisterParticipation(ctx, g.TeamID); err != nil {
			return nil, fmt.Errorf("register participation (team %d): %w", g.TeamID, err)
		}
	}
	if err := s.awardPodium(ctx, graphs[0], model.Gold, summary); err != nil {
		return nil, err
	}
	if err := s.awardPodium(ctx, graphs[1], model.Silver, summary); err != nil {
		return nil, err
	}

	if err := s.registerTournamentRound(ctx, event, graphs[0], "final", []map[string]any{match}); err != nil {
		return nil, err
	}

	summary.Participants = len(graphs)
	summary.Records = 0
	return summary, nil
}

// realizeThirdPlace plays the bronze match: the winner takes bronze.
func (s *Service) realizeThirdPlace(ctx context.Context, event *model.Event, graphs []*model.TeamGraph, summary *model.RealizeSummary) (*model.RealizeSummary, error) {
	if len(graphs) != 2 {
		return nil, fmt.Errorf("tercer_puesto event %d requires exactly 2 teams, got %d", event.ID, len(graphs))
	}
	comps := competitors(graphs)
	match := buildMatch(comps[0], comps[1], "tercer_puesto") // comps[0] wins

	for _, g := range graphs {
		if err := s.RegisterParticipation(ctx, g.TeamID); err != nil {
			return nil, fmt.Errorf("register participation (team %d): %w", g.TeamID, err)
		}
	}
	if err := s.awardPodium(ctx, graphs[0], model.Bronze, summary); err != nil {
		return nil, err
	}

	if err := s.registerTournamentRound(ctx, event, graphs[0], "tercer_puesto", []map[string]any{match}); err != nil {
		return nil, err
	}

	summary.Participants = len(graphs)
	summary.Records = 0
	return summary, nil
}

// createNextRounds creates the two events that follow the semifinal — the bronze
// match (tercer_puesto, +1 day) and the final (+2 days) — both chained back to it
// and starting empty; realizeSemifinal then advances the losers and winners into
// them. Their names reuse the semifinal's, swapping the "Semifinal " prefix.
func (s *Service) createNextRounds(ctx context.Context, semi *model.Event) (final, third *model.Event, err error) {
	base := strings.TrimPrefix(semi.Name, "Semifinal ")
	third = &model.Event{
		GameID:          semi.GameID,
		DisciplineID:    semi.DisciplineID,
		Name:            "Tercer puesto " + base,
		Date:            semi.Date.AddDate(0, 0, 1),
		Phase:           "tercer_puesto",
		PreviousEventID: &semi.ID,
	}
	if err := s.createRoundEvent(ctx, third); err != nil {
		return nil, nil, err
	}
	final = &model.Event{
		GameID:          semi.GameID,
		DisciplineID:    semi.DisciplineID,
		Name:            "Final " + base,
		Date:            semi.Date.AddDate(0, 0, 2),
		Phase:           "final",
		PreviousEventID: &semi.ID,
	}
	if err := s.createRoundEvent(ctx, final); err != nil {
		return nil, nil, err
	}
	return final, third, nil
}

// createRoundEvent persists one tournament round in Postgres and mirrors it into
// the Neo4j graph.
func (s *Service) createRoundEvent(ctx context.Context, e *model.Event) error {
	if err := s.sql.CreateEvent(ctx, e); err != nil {
		return fmt.Errorf("create round event %q: %w", e.Name, err)
	}
	if err := s.graph.UpsertEvent(ctx, e); err != nil {
		return fmt.Errorf("sync round event %q: %w", e.Name, err)
	}
	return nil
}

// advanceTeam enters a team that reached the next round into eventID, copying
// its country participation and full roster so the new team mirrors the old one.
func (s *Service) advanceTeam(ctx context.Context, eventID int64, tg *model.TeamGraph) error {
	gcID, err := s.sql.GetGameCountryID(ctx, tg.GameID, tg.CountryID)
	if err != nil {
		return fmt.Errorf("game country (%d/%d): %w", tg.GameID, tg.CountryID, err)
	}
	team := &model.Team{GameCountryID: gcID, EventID: eventID}
	if err := s.sql.CreateTeam(ctx, team); err != nil {
		return fmt.Errorf("create team: %w", err)
	}
	for _, a := range tg.Athletes {
		if err := s.sql.AddAthleteToTeam(ctx, team.ID, a.ID); err != nil {
			return fmt.Errorf("add athlete %d to team %d: %w", a.ID, team.ID, err)
		}
	}
	return nil
}

// awardPodium awards one medal and records it in the summary.
func (s *Service) awardPodium(ctx context.Context, tg *model.TeamGraph, medal model.MedalType, summary *model.RealizeSummary) error {
	if err := s.AwardMedal(ctx, tg.TeamID, medal); err != nil {
		return fmt.Errorf("award medal (team %d): %w", tg.TeamID, err)
	}
	summary.Medals = append(summary.Medals, fmt.Sprintf("%s -> %s", tg.CountryName, medal))
	return nil
}

// registerTournamentRound writes the Mongo document for one round; the round is
// tagged inside Result ("round") so each round reads as its own input.
func (s *Service) registerTournamentRound(ctx context.Context, event *model.Event, first *model.TeamGraph, phase string, matches []map[string]any) error {
	if err := s.RegisterEventResult(ctx, &model.EventResult{
		EventID:        event.ID,
		EventName:      first.EventName,
		GameID:         first.GameID,
		GameName:       fmt.Sprintf("%s %d", first.GameCity, first.GameYear),
		DisciplineID:   first.DisciplineID,
		DisciplineName: first.DisciplineName,
		Sport:          first.SportName,
		Date:           time.Now(),
		Result: map[string]any{
			"round":   phase,
			"matches": matches,
		},
		Records: []model.RecordMark{}, // football has no olympic record marks
	}); err != nil {
		return fmt.Errorf("register event result (%s): %w", phase, err)
	}
	return nil
}

func competitors(graphs []*model.TeamGraph) []*footballCompetitor {
	comps := make([]*footballCompetitor, len(graphs))
	for i, g := range graphs {
		comps[i] = &footballCompetitor{graph: g}
	}
	return comps
}

// buildMatch invents one match where winner beats loser, with coherent stats
// (possession sums to 100, shots on target >= goals, saves = the opponent's
// on-target shots that did not score) and goals drawn from each roster.
func buildMatch(winner, loser *footballCompetitor, round string) map[string]any {
	winScore := 1 + rand.Intn(3)     // 1..3
	loseScore := rand.Intn(winScore) // 0..winScore-1

	t1 := teamSide(winner, winScore)
	t2 := teamSide(loser, loseScore)

	// Possession sums to 100, slightly favouring the winner.
	possession := 50 + rand.Intn(11) // 50..60
	statsOf(t1)["possessionPct"] = possession
	statsOf(t2)["possessionPct"] = 100 - possession

	statsOf(t1)["saves"] = clampMin(intStat(t2, "shotsOnTarget")-loseScore, 0)
	statsOf(t2)["saves"] = clampMin(intStat(t1, "shotsOnTarget")-winScore, 0)

	goals := append(inventGoals(winner, winScore), inventGoals(loser, loseScore)...)
	sort.Slice(goals, func(i, j int) bool { return goals[i]["minute"].(int) < goals[j]["minute"].(int) })

	return map[string]any{
		"round":        round,
		"team_1":       t1,
		"team_2":       t2,
		"winnerTeamId": winner.teamID(),
		"winner":       winner.name(),
		"fullTime":     fmt.Sprintf("%d-%d", winScore, loseScore),
		"halfTime":     fmt.Sprintf("%d-%d", rand.Intn(winScore+1), rand.Intn(loseScore+1)),
		"attendance":   20000 + rand.Intn(60000),
		"goals":        goals,
	}
}

// teamSide builds one side of a match with its score and statistics. possession
// and saves are filled by buildMatch because they depend on the rival.
func teamSide(c *footballCompetitor, score int) map[string]any {
	shots := 8 + rand.Intn(11) // 8..18
	shotsOnTarget := score
	if shots > score {
		shotsOnTarget = score + rand.Intn(shots-score+1)
	}
	return map[string]any{
		"teamId":      c.teamID(),
		"countryId":   c.countryID(),
		"countryName": c.name(),
		"score":       score,
		"stats": map[string]any{
			"passAccuracyPct": 70 + rand.Intn(21), // 70..90
			"fouls":           6 + rand.Intn(11),  // 6..16
			"shots":           shots,
			"shotsOnTarget":   shotsOnTarget,
			"corners":         2 + rand.Intn(8), // 2..9
			"offsides":        rand.Intn(5),     // 0..4
			"yellowCards":     rand.Intn(4),     // 0..3
			"redCards":        boolToInt(rand.Intn(10) == 0),
		},
	}
}

// inventGoals picks `score` scorers for a team: real athletes when the team has
// a roster, generic names for filler teams.
func inventGoals(c *footballCompetitor, score int) []map[string]any {
	goals := make([]map[string]any, 0, score)
	for i := 0; i < score; i++ {
		goalType := "open_play"
		if rand.Intn(6) == 0 {
			goalType = "penalty"
		}
		id, name := footballScorer(c, i)
		goals = append(goals, map[string]any{
			"athleteId":   id,
			"athleteName": name,
			"teamId":      c.teamID(),
			"countryId":   c.countryID(),
			"minute":      1 + rand.Intn(90),
			"type":        goalType,
		})
	}
	return goals
}

func footballScorer(c *footballCompetitor, i int) (int64, string) {
	if len(c.graph.Athletes) > 0 {
		a := c.graph.Athletes[rand.Intn(len(c.graph.Athletes))]
		return a.ID, a.Name
	}
	return 0, fmt.Sprintf("Jugador %s %d", c.name(), i+1)
}

func statsOf(side map[string]any) map[string]any { return side["stats"].(map[string]any) }

func intStat(side map[string]any, key string) int { return statsOf(side)[key].(int) }

func clampMin(v, lo int) int {
	if v < lo {
		return lo
	}
	return v
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
