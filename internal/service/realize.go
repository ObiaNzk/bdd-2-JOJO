package service

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/ObiaNzk/bdd-2-JOJO/internal/model"
)

// RealizeEvent simulates running an already-set-up event: it draws a random
// finishing order for every team entered and dispatches to a discipline-specific
// builder that invents the result, awards the medals and writes one type-specific
// document to Mongo while fanning results out to Postgres, Redis and Neo4j.
// Setting up entities only touches Postgres; "realizing" the event is what fans
// medals, results and records out across all four stores.
//
// Single-event disciplines (race / field) require at least 3 teams and award a
// full podium. Tournament events are one round each: the team-count requirement
// and the medals depend on the phase (see realizeTournament), so the validation
// for those lives there.
// winnerMark, when non-nil, forces the winning mark of a single-event final
// (the 100m time or the vault height) instead of drawing it at random; the rest
// of the field is scaled relative to it. It lets callers (e.g. the seed) script a
// deterministic record narrative across editions. Tournaments ignore it.
func (s *Service) RealizeEvent(ctx context.Context, eventID int64, winnerMark *float64) (*model.RealizeSummary, error) {
	event, err := s.sql.GetEventByID(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if event.Realized {
		return nil, fmt.Errorf("event %d already realized", eventID)
	}
	teamIDs, err := s.sql.ListTeamsByEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if len(teamIDs) == 0 {
		return nil, fmt.Errorf("event %d has no teams", eventID)
	}

	// Random finishing order.
	rand.Shuffle(len(teamIDs), func(i, j int) { teamIDs[i], teamIDs[j] = teamIDs[j], teamIDs[i] })

	graphs := make([]*model.TeamGraph, len(teamIDs))
	for i, id := range teamIDs {
		tg, err := s.sql.GetTeamGraph(ctx, id)
		if err != nil {
			return nil, err
		}
		graphs[i] = tg
	}

	first := graphs[0]
	// resultFormat decides which builder simulates the event; it is internal
	// dispatch only and is not persisted in the Mongo document.
	format := resultFormat(first.DisciplineName)
	summary := &model.RealizeSummary{
		EventID:        eventID,
		EventName:      first.EventName,
		DisciplineName: first.DisciplineName,
		Sport:          first.SportName,
	}

	// Each discipline invents its own type-specific Mongo document:
	//   - tournament -> one knockout round     (realize_tournament.go)
	//   - swimming   -> a timed final          (realize_swimming.go)
	//   - pole vault -> a field-attempt card   (realize_vault.go)
	var out *model.RealizeSummary
	switch format {
	case "tournament":
		out, err = s.realizeTournament(ctx, event, graphs, summary)
	case "race":
		if len(graphs) < 3 {
			return nil, fmt.Errorf("event %d requires at least 3 teams, got %d", eventID, len(graphs))
		}
		out, err = s.realizeSwimming(ctx, event, graphs, summary, winnerMark)
	case "field_attempts":
		if len(graphs) < 3 {
			return nil, fmt.Errorf("event %d requires at least 3 teams, got %d", eventID, len(graphs))
		}
		out, err = s.realizeVault(ctx, event, graphs, summary, winnerMark)
	default:
		return nil, fmt.Errorf("unknown result format %q for discipline %q", format, first.DisciplineName)
	}
	if err != nil {
		return nil, err
	}

	if err := s.sql.MarkEventRealized(ctx, eventID); err != nil {
		return nil, fmt.Errorf("mark event realized: %w", err)
	}
	return out, nil
}

// ResultFormat reports the result shape for a discipline name ("tournament",
// "field_attempts" or "race"). Exported so callers such as the console can tell
// tournament disciplines apart (to create their chained rounds) before realizing.
func ResultFormat(discipline string) string { return resultFormat(discipline) }

// resultFormat picks the Mongo document shape from the discipline name: football
// is a knockout tournament, pole vault / jumps record height attempts, and
// everything else is a timed race.
func resultFormat(discipline string) string {
	d := strings.ToLower(discipline)
	switch {
	case strings.Contains(d, "fútbol"), strings.Contains(d, "futbol"), strings.Contains(d, "football"):
		return "tournament"
	case strings.Contains(d, "garrocha"), strings.Contains(d, "salto"), strings.Contains(d, "vault"), strings.Contains(d, "jump"):
		return "field_attempts"
	default:
		return "race"
	}
}
