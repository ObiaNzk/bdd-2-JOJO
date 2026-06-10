package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ObiaNzk/bdd-2-JOJO/internal/model"
)

func (h *Handler) Router() chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(15 * time.Second))

	r.Get("/healthz", h.health)

	// Writes (fan out from Postgres to the derived stores).
	r.Post("/medals", h.awardMedal)
	r.Post("/event-results", h.registerEventResult)

	// Realize an event: invents a discipline-specific result, awards medals and
	// fans everything out across the four stores.
	r.Post("/events/{eventID}/realize", h.realizeEvent)

	// Use case 1: medals per country in the latest games (Redis).
	r.Get("/games/latest/medals", h.medalRankingLatest)

	// Use case 2: athletes that hold olympic records, derived from the
	// event results in Mongo (optionally filtered by ?discipline=).
	r.Get("/records", h.records)

	// World-record history: the standing world record per discipline+metric, with
	// the full chronological timeline of holders (Mongo `world_records`).
	r.Get("/world-records", h.worldRecords)

	// Raw, type-specific event-result documents (Mongo).
	r.Get("/event-results", h.eventResults)

	// Use case 3: most popular events (Postgres exact + Redis per-event HLL).
	r.Get("/games/{gameID}/events/popular", h.popularEvents)
	r.Get("/events/{eventID}/popularity", h.eventPopularity)

	// Use case 4: athletes with medals in multiple disciplines (Neo4j).
	r.Get("/athletes/multi-discipline", h.multiDiscipline)

	// Use case 5: host country per edition (Postgres).
	r.Get("/games/{gameID}/host", h.hostByGame)
	r.Get("/hosts", h.hosts)

	// Use case 6: medals of a country per discipline, across every edition (Neo4j).
	r.Get("/countries/{countryID}/medals-by-discipline", h.medalsByCountryAndDiscipline)

	// Use case 7: athletes with >N medals OR a standing olympic record, across every edition (Redis + Mongo).
	r.Get("/top-athletes", h.topAthletes)

	return r
}

// --- Writes ---

func (h *Handler) awardMedal(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TeamID int64           `json:"teamId"`
		Type   model.MedalType `json:"type"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.svc.AwardMedal(r.Context(), body.TeamID, body.Type); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "awarded"})
}

func (h *Handler) registerEventResult(w http.ResponseWriter, r *http.Request) {
	var res model.EventResult
	if err := decode(r, &res); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.svc.RegisterEventResult(r.Context(), &res); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (h *Handler) realizeEvent(w http.ResponseWriter, r *http.Request) {
	eventID, err := urlInt(r, "eventID")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// Optional ?winnerMark= forces the winning mark (e.g. for a scripted record
	// scenario); absent, the service draws it at random as usual.
	var winnerMark *float64
	if v := r.URL.Query().Get("winnerMark"); v != "" {
		mark, err := strconv.ParseFloat(v, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		winnerMark = &mark
	}
	summary, err := h.svc.RealizeEvent(r.Context(), eventID, winnerMark)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, summary)
}

// --- Reads ---

func (h *Handler) medalRankingLatest(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.MedalRankingLatest(r.Context(), queryInt(r, "limit", 10))
	respond(w, out, err)
}

func (h *Handler) records(w http.ResponseWriter, r *http.Request) {
	if id := r.URL.Query().Get("discipline"); id != "" {
		disciplineID, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		out, err := h.svc.RecordHoldersByDiscipline(r.Context(), disciplineID)
		respond(w, out, err)
		return
	}
	out, err := h.svc.RecordHolders(r.Context())
	respond(w, out, err)
}

func (h *Handler) worldRecords(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.WorldRecords(r.Context())
	respond(w, out, err)
}

func (h *Handler) eventResults(w http.ResponseWriter, r *http.Request) {
	if id := r.URL.Query().Get("discipline"); id != "" {
		disciplineID, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		out, err := h.svc.EventResultsByDiscipline(r.Context(), disciplineID)
		respond(w, out, err)
		return
	}
	out, err := h.svc.EventResults(r.Context())
	respond(w, out, err)
}

func (h *Handler) popularEvents(w http.ResponseWriter, r *http.Request) {
	gameID, err := urlInt(r, "gameID")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := h.svc.PopularEvents(r.Context(), gameID, queryInt(r, "limit", 10))
	respond(w, out, err)
}

func (h *Handler) eventPopularity(w http.ResponseWriter, r *http.Request) {
	eventID, err := urlInt(r, "eventID")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	count, err := h.svc.EventPopularity(r.Context(), eventID)
	respond(w, map[string]int64{"countriesCount": count}, err)
}

func (h *Handler) multiDiscipline(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.AthletesInMultipleDisciplines(r.Context(), queryInt(r, "min", 2))
	respond(w, out, err)
}

func (h *Handler) hostByGame(w http.ResponseWriter, r *http.Request) {
	gameID, err := urlInt(r, "gameID")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := h.svc.HostByGame(r.Context(), gameID)
	respond(w, out, err)
}

func (h *Handler) hosts(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Hosts(r.Context())
	respond(w, out, err)
}

func (h *Handler) medalsByCountryAndDiscipline(w http.ResponseWriter, r *http.Request) {
	countryID, err := urlInt(r, "countryID")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := h.svc.MedalsByCountryAndDiscipline(r.Context(), countryID)
	respond(w, out, err)
}

func (h *Handler) topAthletes(w http.ResponseWriter, r *http.Request) {
	// minMedals defaults to 4 to express ">3 medals".
	out, err := h.svc.TopAthletes(r.Context(), queryInt(r, "min", 4))
	respond(w, out, err)
}
