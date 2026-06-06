package service

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/ObiaNzk/bdd-2-JOJO/internal/model"
)

// realizeSwimming invents the final of a timed race (e.g. 100m Libre) and stores
// it as one Mongo document: a ranking of swimmers by finishing position, each
// with lane, reaction time, the 50m split and the final time (all in seconds).
// The winner carries the olympic record. graphs are already in random finishing
// order.
func (s *Service) realizeSwimming(ctx context.Context, event *model.Event, graphs []*model.TeamGraph, summary *model.RealizeSummary) (*model.RealizeSummary, error) {
	podium := []model.MedalType{model.Gold, model.Silver, model.Bronze}
	lanes := swimLanes(len(graphs))
	ranking := make([]map[string]any, 0, len(graphs))
	var records []model.RecordMark

	for pos, tg := range graphs {
		position := pos + 1
		timeS := fakeSwimTimeS(position)
		split50S := round2(timeS * (0.475 + rand.Float64()*0.01)) // first 50 is faster (dive)
		reactionS := round2(0.60 + rand.Float64()*0.10)           // 0.60..0.70

		if err := s.RegisterParticipation(ctx, tg.TeamID); err != nil {
			return nil, fmt.Errorf("register participation (team %d): %w", tg.TeamID, err)
		}

		if pos < len(podium) {
			if err := s.AwardMedal(ctx, tg.TeamID, podium[pos]); err != nil {
				return nil, fmt.Errorf("award medal (team %d): %w", tg.TeamID, err)
			}
			summary.Medals = append(summary.Medals, fmt.Sprintf("%s -> %s", tg.CountryName, podium[pos]))
		}

		for _, a := range tg.Athletes {
			ranking = append(ranking, map[string]any{
				"rank":        position,
				"athleteId":   a.ID,
				"athleteName": a.Name,
				"teamId":      tg.TeamID,
				"countryId":   a.CountryID,
				"lane":        lanes[pos],
				"reactionS":   reactionS,
				"splitS":      []float64{split50S, timeS},
				"timeS":       timeS,
			})
			if position == 1 {
				records = append(records, model.RecordMark{
					AthleteID:   a.ID,
					AthleteName: a.Name,
					Type:        "OR",
					Metric:      "time_s",
					Value:       timeS,
				})
			}
		}
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
		Date:           time.Now(),
		Result: map[string]any{
			"poolLengthM": 50,
			"ranking":     ranking,
		},
		Records: records,
	}); err != nil {
		return nil, fmt.Errorf("register event result: %w", err)
	}

	summary.Participants = len(ranking)
	summary.Records = len(records)
	return summary, nil
}

// fakeSwimTimeS returns a plausible 100m freestyle time in seconds that grows
// with the finishing position (~46.9s for the winner).
func fakeSwimTimeS(position int) float64 {
	return round2(46.90 + float64(position-1)*0.45 + rand.Float64()*0.15)
}

// swimLanes returns a unique lane per finishing position, seeded centre-out
// (4,5,3,6,...) like a real final and then shuffled so the lane is independent
// of who wins.
func swimLanes(n int) []int {
	order := []int{4, 5, 3, 6, 2, 7, 1, 8}
	lanes := make([]int, 0, n)
	for i := 0; i < n && i < len(order); i++ {
		lanes = append(lanes, order[i])
	}
	for l := 9; len(lanes) < n; l++ { // overflow past 8 lanes (uncommon)
		lanes = append(lanes, l)
	}
	rand.Shuffle(len(lanes), func(i, j int) { lanes[i], lanes[j] = lanes[j], lanes[i] })
	return lanes
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
