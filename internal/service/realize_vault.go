package service

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/ObiaNzk/bdd-2-JOJO/internal/model"
)

// vaultBarStep is the spacing of the bar progression, in metres.
const vaultBarStep = 0.05

// realizeVault invents the final of a field event (Salto con garrocha) and
// stores it as one Mongo document: a shared bar progression plus, per athlete,
// the best height cleared and the attempt card at every contested bar
// ("O" cleared, "X" failed, "-" passed), all in metres. Ranking is by best
// height (descending, no countback). The winner carries the olympic record.
// graphs are already in random finishing order.
func (s *Service) realizeVault(ctx context.Context, event *model.Event, graphs []*model.TeamGraph, summary *model.RealizeSummary) (*model.RealizeSummary, error) {
	podium := []model.MedalType{model.Gold, model.Silver, model.Bronze}
	n := len(graphs)

	// Best heights: distinct and decreasing by finishing position.
	winnerBest := snap05(5.95 + rand.Float64()*0.20) // 5.95..6.15
	bests := make([]float64, n)
	for i := range bests {
		bests[i] = round2(winnerBest - float64(i)*vaultBarStep)
	}
	failBar := round2(winnerBest + vaultBarStep)

	// Shared ascending bar progression: lowest best .. winner's best .. fail bar.
	barHeights := make([]float64, 0, n+1)
	for i := n - 1; i >= 0; i-- {
		barHeights = append(barHeights, bests[i])
	}
	barHeights = append(barHeights, failBar)

	ranking := make([]map[string]any, 0, len(graphs))
	var records []model.RecordMark

	for pos, tg := range graphs {
		position := pos + 1
		best := bests[pos]

		if err := s.RegisterParticipation(ctx, tg.TeamID); err != nil {
			return nil, fmt.Errorf("register participation (team %d): %w", tg.TeamID, err)
		}

		if pos < len(podium) {
			if err := s.AwardMedal(ctx, tg.TeamID, podium[pos]); err != nil {
				return nil, fmt.Errorf("award medal (team %d): %w", tg.TeamID, err)
			}
			summary.Medals = append(summary.Medals, fmt.Sprintf("%s -> %s", tg.CountryName, podium[pos]))
		}

		attempts := vaultAttempts(barHeights, best)
		for _, a := range tg.Athletes {
			ranking = append(ranking, map[string]any{
				"rank":        position,
				"athleteId":   a.ID,
				"athleteName": a.Name,
				"teamId":      tg.TeamID,
				"countryId":   a.CountryID,
				"bestHeightM": best,
				"attempts":    attempts,
			})
			if position == 1 {
				records = append(records, model.RecordMark{
					AthleteID:   a.ID,
					AthleteName: a.Name,
					Type:        "OR",
					Metric:      "height_m",
					Value:       best,
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
			"barHeightsM": barHeights,
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

// vaultAttempts builds the attempt card for an athlete whose best cleared height
// is best: bars below are cleared (sometimes passed), the best is cleared, and
// the first bar above is failed three times (eliminated).
func vaultAttempts(bars []float64, best float64) []map[string]any {
	attempts := make([]map[string]any, 0, len(bars))
	for _, h := range bars {
		switch {
		case h < best-0.001:
			attempts = append(attempts, vaultAttempt(h, clearedOrPassed()))
		case h <= best+0.001:
			attempts = append(attempts, vaultAttempt(h, clearedResults()))
		default:
			attempts = append(attempts, vaultAttempt(h, []string{"X", "X", "X"}))
			return attempts
		}
	}
	return attempts
}

func vaultAttempt(height float64, results []string) map[string]any {
	return map[string]any{"heightM": height, "results": results}
}

// clearedResults is the attempt sequence ending in a clear, with the occasional
// miss before it.
func clearedResults() []string {
	switch r := rand.Intn(100); {
	case r < 70:
		return []string{"O"}
	case r < 95:
		return []string{"X", "O"}
	default:
		return []string{"X", "X", "O"}
	}
}

// clearedOrPassed is a cleared lower bar that the athlete sometimes skips.
func clearedOrPassed() []string {
	if rand.Intn(5) == 0 {
		return []string{"-"}
	}
	return clearedResults()
}

func snap05(v float64) float64 { return round2(math.Round(v/0.05) * 0.05) }
