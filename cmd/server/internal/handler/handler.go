// Package handler contains the HTTP handlers for the `server` binary.
// It declares the Service interface it consumes and receives an
// implementation by constructor.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ObiaNzk/bdd-2-JOJO/internal/model"
)

// Service is the interface the handler consumes from the service layer.
type Service interface {
	AwardMedal(ctx context.Context, teamID int64, medalType model.MedalType) error
	RegisterEventResult(ctx context.Context, res *model.EventResult) error
	RealizeEvent(ctx context.Context, eventID int64) (*model.RealizeSummary, error)

	MedalRankingLatest(ctx context.Context, limit int) ([]model.MedalCount, error)
	RecordHolders(ctx context.Context) ([]model.RecordHolder, error)
	RecordHoldersByDiscipline(ctx context.Context, disciplineID int64) ([]model.RecordHolder, error)
	EventResults(ctx context.Context) ([]model.EventResult, error)
	EventResultsByDiscipline(ctx context.Context, disciplineID int64) ([]model.EventResult, error)
	PopularEvents(ctx context.Context, gameID int64, limit int) ([]model.EventPopularity, error)
	EventPopularity(ctx context.Context, eventID int64) (int64, error)
	AthletesInMultipleDisciplines(ctx context.Context, minDisciplines int) ([]model.AthleteDisciplines, error)
	HostByGame(ctx context.Context, gameID int64) (*model.HostInfo, error)
	Hosts(ctx context.Context) ([]model.HostInfo, error)
	MedalsByCountryAndDiscipline(ctx context.Context, countryID int64) ([]model.DisciplineMedalCount, error)
	TopAthletes(ctx context.Context, minMedals int) ([]model.TopAthlete, error)
}

type Handler struct {
	svc Service
}

func New(svc Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// respond writes v as 200 JSON, or maps a non-nil error to 500.
func respond(w http.ResponseWriter, v any, err error) {
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func decode(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func urlInt(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, key), 10, 64)
}

func queryInt(r *http.Request, key string, fallback int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
