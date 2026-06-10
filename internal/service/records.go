package service

import (
	"context"
	"fmt"

	"github.com/ObiaNzk/bdd-2-JOJO/internal/model"
)

// Record metrics and their "better" direction. A swimming time is better when
// lower; a vault height is better when higher.
const (
	MetricTimeS   = "time_s"
	MetricHeightM = "height_m"
)

// metricDirection reports which way is better for a metric: "lower" for a time,
// "higher" for a height/distance. Unknown metrics default to "higher".
func metricDirection(metric string) string {
	if metric == MetricTimeS {
		return "lower"
	}
	return "higher"
}

// betterMark reports whether candidate beats standing given the metric direction.
// A strict improvement is required, so merely equalling the record does not break
// it.
func betterMark(direction string, candidate, standing float64) bool {
	if direction == "lower" {
		return candidate < standing
	}
	return candidate > standing
}

// flattenRecordHolders turns the world_records ledgers into the case-2 projection:
// the standing olympic record of each discipline — the current holder only, which
// is the first entry of the newest-first timeline. When disciplineID is non-zero
// it keeps only that discipline.
func flattenRecordHolders(wrs []model.WorldRecord, disciplineID int64) []model.RecordHolder {
	out := make([]model.RecordHolder, 0)
	for _, wr := range wrs {
		if disciplineID != 0 && wr.DisciplineID != disciplineID {
			continue
		}
		if len(wr.History) == 0 {
			continue
		}
		h := wr.History[0] // standing record (newest-first)
		out = append(out, model.RecordHolder{
			AthleteID:      h.AthleteID,
			AthleteName:    h.AthleteName,
			DisciplineID:   wr.DisciplineID,
			DisciplineName: wr.DisciplineName,
			Sport:          wr.Sport,
			EventID:        h.EventID,
			GameName:       h.GameName,
			Type:           "OR",
			Metric:         wr.Metric,
			Value:          h.Value,
		})
	}
	return out
}

// evaluateEventRecord is the shared bridge the single-event builders (swimming,
// vault) use after they have drawn a result: it turns the winning team's mark
// into a world-record candidate, runs it through the ledger, then reflects the
// outcome back into the event's record marks and the realize summary. It returns
// the world record that stands afterwards and whether this edition broke it, so
// the caller can store them on the EventResult document.
func (s *Service) evaluateEventRecord(ctx context.Context, winner *model.TeamGraph, event *model.Event, metric string, winnerValue float64, records []model.RecordMark, summary *model.RealizeSummary) (model.WorldRecordHolder, bool, error) {
	if len(winner.Athletes) == 0 {
		return model.WorldRecordHolder{}, false, fmt.Errorf("winning team %d has no athletes", winner.TeamID)
	}
	a := winner.Athletes[0]
	candidate := model.WorldRecordHolder{
		AthleteID:   a.ID,
		AthleteName: a.Name,
		CountryID:   winner.CountryID,
		CountryName: winner.CountryName,
		EventID:     event.ID,
		GameID:      winner.GameID,
		GameName:    fmt.Sprintf("%s %d", winner.GameCity, winner.GameYear),
		Metric:      metric,
		Value:       winnerValue,
		// Date is the game's own date so the validity timeline reads as the
		// olympic editions themselves, not the wall-clock of the simulation.
		Date: event.Date,
	}

	standing, broken, err := s.EvaluateWorldRecord(ctx, candidate, winner.DisciplineID, winner.DisciplineName, winner.SportName)
	if err != nil {
		return model.WorldRecordHolder{}, false, err
	}

	// Every record is an olympic record ("OR"); the records slice already carries
	// that type. Only when this mark beats the standing one does the holder change.
	if broken {
		summary.WorldRecord = fmt.Sprintf("Record olímpico roto por %s (%s): %.2f %s", standing.AthleteName, standing.CountryName, standing.Value, standing.Metric)

		// Neo4j must hold only the current record holder per discipline: drop the
		// superseded holder's node, then attach the new one(s).
		if err := s.graph.DeleteDisciplineRecord(ctx, winner.DisciplineID); err != nil {
			return model.WorldRecordHolder{}, false, fmt.Errorf("delete old discipline record: %w", err)
		}
		for _, rm := range records {
			if err := s.graph.LinkAthleteRecord(ctx, rm.AthleteID, winner.TeamID, event.ID, winner.DisciplineID, rm.Metric, rm.Value); err != nil {
				return model.WorldRecordHolder{}, false, fmt.Errorf("link athlete record: %w", err)
			}
		}
	} else {
		// The mark did not beat the standing record, so the graph holder is left
		// untouched (this edition's winner does not become a record holder).
		summary.WorldRecord = fmt.Sprintf("Record vigente de %s: %.2f %s (%s)", standing.AthleteName, standing.Value, standing.Metric, standing.GameName)
	}
	return standing, broken, nil
}

// EvaluateWorldRecord compares a just-realized winning mark (candidate) against
// the standing world record for its discipline+metric and updates the ledger.
//
// It returns the world record that stands afterwards and whether it was broken:
//   - No ledger yet, or candidate beats the standing holder -> the candidate
//     becomes the new holder: its SetAt is stamped from its Date, it is prepended
//     to the History timeline (kept newest-first, so the standing record is always
//     the first entry), the ledger is upserted, and (candidate, true) is returned.
//   - Otherwise the past holder still stands: the ledger is left untouched and
//     (standingHolder, false) is returned.
func (s *Service) EvaluateWorldRecord(ctx context.Context, candidate model.WorldRecordHolder, disciplineID int64, disciplineName, sport string) (model.WorldRecordHolder, bool, error) {
	direction := metricDirection(candidate.Metric)

	wr, err := s.results.GetWorldRecord(ctx, disciplineID, candidate.Metric)
	if err != nil {
		return model.WorldRecordHolder{}, false, fmt.Errorf("get world record: %w", err)
	}

	// An existing ledger always carries at least one holder; the standing record
	// is the first entry of the timeline (newest-first order).
	if wr != nil && len(wr.History) > 0 {
		standing := wr.History[0]
		if !betterMark(direction, candidate.Value, standing.Value) {
			return standing, false, nil
		}
		candidate.SetAt = candidate.Date
		wr.History = append([]model.WorldRecordHolder{candidate}, wr.History...)
		if err := s.results.UpsertWorldRecord(ctx, wr); err != nil {
			return model.WorldRecordHolder{}, false, fmt.Errorf("upsert world record: %w", err)
		}
		return candidate, true, nil
	}

	// First mark ever recorded for this discipline+metric: it inaugurates the record.
	candidate.SetAt = candidate.Date
	newWR := &model.WorldRecord{
		DisciplineID:   disciplineID,
		DisciplineName: disciplineName,
		Sport:          sport,
		Metric:         candidate.Metric,
		Direction:      direction,
		History:        []model.WorldRecordHolder{candidate},
	}
	if err := s.results.UpsertWorldRecord(ctx, newWR); err != nil {
		return model.WorldRecordHolder{}, false, fmt.Errorf("upsert world record: %w", err)
	}
	return candidate, true, nil
}
