package service

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/ObiaNzk/bdd-2-JOJO/internal/model"
)

// footballBracketSize is the number of teams in the invented knockout: a full
// round of 16 (octavos -> cuartos -> semifinal -> final) so every football
// event tells the same tournament story.
const footballBracketSize = 16

// footballCompetitor is one team in the bracket. It always carries a TeamGraph
// from a real entered team — when fewer than 16 teams are registered, the
// bracket is padded with random copies of those same teams so every match
// references only countries that actually participated.
type footballCompetitor struct {
	graph *model.TeamGraph
}

func (c *footballCompetitor) teamID() int64    { return c.graph.TeamID }
func (c *footballCompetitor) countryID() int64 { return c.graph.CountryID }
func (c *footballCompetitor) name() string     { return c.graph.CountryName }

// realizeFootball invents a full knockout tournament for a football event and
// stores it as one Mongo document. The four semifinalists define the medals:
// champion -> gold, finalist -> silver, winner of the bronze match -> bronze.
// When fewer than 16 teams are entered, the early rounds are padded with copies
// of those same teams so every match references a participating country.
func (s *Service) realizeFootball(ctx context.Context, event *model.Event, graphs []*model.TeamGraph, summary *model.RealizeSummary) (*model.RealizeSummary, error) {
	field := buildFootballField(graphs)

	// Front of the (already shuffled) field are the semifinalists; the rest lose
	// before the semis.
	semifinalists := field[:4]
	others := field[4:]

	champion, runnerUp, bronze, fourth := semifinalists[0], semifinalists[1], semifinalists[2], semifinalists[3]

	// Each semifinalist heads a 4-team quadrant (itself + 3 others). Octavos: the
	// semifinalist beats one rival and a second rival beats the third; cuartos:
	// the semifinalist beats that second rival.
	octavos := make([]map[string]any, 0, 8)
	cuartos := make([]map[string]any, 0, 4)
	for qi, sf := range semifinalists {
		grp := others[qi*3 : qi*3+3]
		octavos = append(octavos, buildMatch(sf, grp[0], "octavos"))
		octavos = append(octavos, buildMatch(grp[1], grp[2], "octavos"))
		cuartos = append(cuartos, buildMatch(sf, grp[1], "cuartos"))
	}

	semifinal := []map[string]any{
		buildMatch(champion, fourth, "semifinal"),
		buildMatch(runnerUp, bronze, "semifinal"),
	}
	final := []map[string]any{buildMatch(champion, runnerUp, "final")}
	tercerPuesto := []map[string]any{buildMatch(bronze, fourth, "tercer_puesto")}

	rounds := []map[string]any{
		{"round": "octavos", "matches": octavos},
		{"round": "cuartos", "matches": cuartos},
		{"round": "semifinal", "matches": semifinal},
		{"round": "final", "matches": final},
		{"round": "tercer_puesto", "matches": tercerPuesto},
	}

	// Each registered team participates once (graphs has no duplicates even if
	// the field does, because the field is padded with copies for the bracket).
	for _, g := range graphs {
		if err := s.RegisterParticipation(ctx, g.TeamID); err != nil {
			return nil, fmt.Errorf("register participation (team %d): %w", g.TeamID, err)
		}
	}
	// Podium goes to the top three semifinalists, which always sit in the front
	// (real) slots of the field.
	medals := map[*footballCompetitor]model.MedalType{champion: model.Gold, runnerUp: model.Silver, bronze: model.Bronze}
	for _, c := range field {
		medal, ok := medals[c]
		if !ok {
			continue
		}
		if err := s.AwardMedal(ctx, c.graph.TeamID, medal); err != nil {
			return nil, fmt.Errorf("award medal (team %d): %w", c.graph.TeamID, err)
		}
		summary.Medals = append(summary.Medals, fmt.Sprintf("%s -> %s", c.graph.CountryName, medal))
	}

	first := graphs[0]
	if err := s.RegisterEventResult(ctx, &model.EventResult{
		EventID:        event.ID,
		EventName:      first.EventName,
		GameID:         first.GameID,
		GameName:       fmt.Sprintf("%s %d", first.GameCity, first.GameYear),
		DisciplineID:   first.DisciplineID,
		DisciplineName: first.DisciplineName,
		Sport:          first.SportName,
		Format:         summary.Format,
		Date:           time.Now(),
		Result: map[string]any{
			"bracketSize": footballBracketSize,
			"rounds":      rounds,
		},
		Records: []model.RecordMark{}, // football has no olympic record marks
	}); err != nil {
		return nil, fmt.Errorf("register event result: %w", err)
	}

	summary.Participants = len(graphs)
	summary.Records = 0
	return summary, nil
}

// buildFootballField returns exactly footballBracketSize competitors. The real
// teams (already shuffled by the caller) take the front slots — that's where
// the semifinalists / podium come from — and any remaining slots are padded
// with random samples of the same real teams so every match in the bracket is
// played between countries that actually participated in the event.
func buildFootballField(graphs []*model.TeamGraph) []*footballCompetitor {
	field := make([]*footballCompetitor, 0, footballBracketSize)
	for _, g := range graphs {
		field = append(field, &footballCompetitor{graph: g})
		if len(field) == footballBracketSize {
			return field
		}
	}
	for len(field) < footballBracketSize {
		field = append(field, &footballCompetitor{graph: graphs[rand.Intn(len(graphs))]})
	}
	return field
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
