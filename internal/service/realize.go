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
// An event spans the whole competition up to the final, so it always awards a
// podium: it requires at least 3 teams, failing up front before writing anything.
func (s *Service) RealizeEvent(ctx context.Context, eventID int64) (*model.RealizeSummary, error) {
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
	if len(teamIDs) < 3 {
		return nil, fmt.Errorf("event %d requires at least 3 teams, got %d", eventID, len(teamIDs))
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
	format := resultFormat(first.DisciplineName)
	summary := &model.RealizeSummary{
		EventID:        eventID,
		EventName:      first.EventName,
		DisciplineName: first.DisciplineName,
		Sport:          first.SportName,
		Format:         format,
	}

	// Each discipline invents its own type-specific Mongo document:
	//   - football -> a knockout tournament   (realize_football.go)
	//   - swimming -> a timed final           (realize_swimming.go)
	//   - pole vault -> a field-attempt card   (realize_vault.go)
	var out *model.RealizeSummary
	switch format {
	case "tournament":
		out, err = s.realizeFootball(ctx, event, graphs, summary)
	case "race":
		out, err = s.realizeSwimming(ctx, event, graphs, summary)
	case "field_attempts":
		out, err = s.realizeVault(ctx, event, graphs, summary)
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
