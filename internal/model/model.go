// Package model holds the plain data structures shared across layers.
// These are data carriers only (no behaviour); every layer maps to and
// from them. Postgres is the source of truth; Redis, Mongo and Neo4j
// store derived views of this same data.
package model

import "time"

type MedalType string

const (
	Gold   MedalType = "gold"
	Silver MedalType = "silver"
	Bronze MedalType = "bronze"
)

type Country struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type OlympicGame struct {
	ID            int64  `json:"id"`
	Year          int    `json:"year"`
	City          string `json:"city"`
	HostCountryID int64  `json:"hostCountryId"`
}

type Sport struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Discipline struct {
	ID      int64  `json:"id"`
	SportID int64  `json:"sportId"`
	Name    string `json:"name"`
}

type Athlete struct {
	ID        int64  `json:"id"`
	CountryID int64  `json:"countryId"`
	Name      string `json:"name"`
}

type Event struct {
	ID           int64     `json:"id"`
	GameID       int64     `json:"gameId"`
	DisciplineID int64     `json:"disciplineId"`
	Name         string    `json:"name"`
	Date         time.Time `json:"date"`
	// Phase names the tournament round when the event is one ("semifinal",
	// "final", "tercer_puesto"); it is empty for single-event disciplines.
	// PreviousEventID chains a round back to the one it advances from (the final
	// and the bronze match both point to their semifinal); it is nil otherwise.
	Phase           string `json:"phase,omitempty"`
	PreviousEventID *int64 `json:"previousEventId,omitempty"`
	// Realized is set once the event has been realized, so the fan-out cannot run
	// twice for the same event. Only the deciding rounds (final, tercer_puesto)
	// award medals; the earlier rounds just advance teams to the next event.
	Realized bool `json:"realized"`
}

type Team struct {
	ID            int64 `json:"id"`
	GameCountryID int64 `json:"gameCountryId"`
	EventID       int64 `json:"eventId"`
}

type GameCountry struct {
	ID        int64 `json:"id"`
	GameID    int64 `json:"gameId"`
	CountryID int64 `json:"countryId"`
}

type Medal struct {
	ID     int64     `json:"id"`
	TeamID int64     `json:"teamId"`
	Type   MedalType `json:"type"`
}

// EventResult is the Mongo document: the full, type-specific outcome of one
// olympic event. Result is intentionally schema-flexible because each discipline
// records its outcome differently (finishing order for a race, attempts per
// athlete for a field event, judged scores for gymnastics...). Persons are
// embedded as snapshots inside Result so the document reads on its own without
// joining back to Postgres.
type EventResult struct {
	ID             string         `json:"id" bson:"_id,omitempty"`
	EventID        int64          `json:"eventId" bson:"eventId"`
	EventName      string         `json:"eventName" bson:"eventName"`
	GameID         int64          `json:"gameId" bson:"gameId"`
	GameName       string         `json:"gameName" bson:"gameName"`
	DisciplineID   int64          `json:"disciplineId" bson:"disciplineId"`
	DisciplineName string         `json:"disciplineName" bson:"disciplineName"`
	Sport          string         `json:"sport" bson:"sport"`
	Date           time.Time      `json:"date" bson:"date"`
	Result         map[string]any `json:"result" bson:"result"`
	// Records is present only when this edition's winning mark set a new olympic
	// record (it beat the standing one); its presence alone signals that.
	Records []RecordMark `json:"records,omitempty" bson:"records,omitempty"`
}

// RecordMark flags, in a queryable top-level array, which embedded athlete set
// an olympic record in this event. Keeping it out of the free-form Result lets
// the record use cases (case 2 and case 7) query without knowing each Format's
// internal shape.
type RecordMark struct {
	AthleteID   int64   `json:"athleteId" bson:"athleteId"`
	AthleteName string  `json:"athleteName" bson:"athleteName"`
	Type        string  `json:"record_type" bson:"record_type"`
	Metric      string  `json:"metric" bson:"metric"`
	Value       float64 `json:"value" bson:"value"`
}

// WorldRecord is the standing-olympic-record ledger for one discipline, kept
// across editions in the Mongo `olympic_records` collection and keyed by
// (DisciplineID, Metric). Its heart is History: the full timeline of every
// athlete that ever held this record, newest first. The current record is simply
// the first entry — it is not stored separately. Direction tells the comparison
// which way is better ("lower" for a time, "higher" for a height).
type WorldRecord struct {
	ID             string              `json:"id" bson:"_id,omitempty"`
	DisciplineID   int64               `json:"disciplineId" bson:"disciplineId"`
	DisciplineName string              `json:"disciplineName" bson:"disciplineName"`
	Sport          string              `json:"sport" bson:"sport"`
	Metric         string              `json:"metric" bson:"metric"`
	Direction      string              `json:"direction" bson:"direction"`
	History        []WorldRecordHolder `json:"history" bson:"history"`
}

// WorldRecordHolder is one entry in a WorldRecord timeline: the full data of the
// mark and who set it. SetAt is when it became the record (the start of its
// validity); the end of its validity is the SetAt of the preceding holder in
// History (the next-newer entry), so it is derived from the sequence rather than
// stored. The first holder is still in effect.
type WorldRecordHolder struct {
	AthleteID   int64     `json:"athleteId" bson:"athleteId"`
	AthleteName string    `json:"athleteName" bson:"athleteName"`
	CountryID   int64     `json:"countryId" bson:"countryId"`
	CountryName string    `json:"countryName" bson:"countryName"`
	EventID     int64     `json:"eventId" bson:"eventId"`
	GameID      int64     `json:"gameId" bson:"gameId"`
	GameName    string    `json:"gameName" bson:"gameName"`
	Metric      string    `json:"metric" bson:"metric"`
	Value       float64   `json:"value" bson:"value"`
	Date        time.Time `json:"date" bson:"date"`
	SetAt       time.Time `json:"setAt" bson:"setAt"`
}

// --- Query result DTOs ---

type MedalCount struct {
	CountryID   int64  `json:"countryId"`
	CountryName string `json:"countryName"`
	Gold        int    `json:"gold"`
	Silver      int    `json:"silver"`
	Bronze      int    `json:"bronze"`
	Total       int    `json:"total"`
}

type DisciplineMedalCount struct {
	DisciplineID   int64  `json:"disciplineId"`
	DisciplineName string `json:"disciplineName"`
	Gold           int    `json:"gold"`
	Silver         int    `json:"silver"`
	Bronze         int    `json:"bronze"`
	Total          int    `json:"total"`
}

type EventPopularity struct {
	EventID        int64  `json:"eventId"`
	EventName      string `json:"eventName"`
	CountriesCount int64  `json:"countriesCount"`
}

type AthleteDisciplines struct {
	AthleteID       int64    `json:"athleteId"`
	AthleteName     string   `json:"athleteName"`
	DisciplineCount int      `json:"disciplineCount"`
	Disciplines     []string `json:"disciplines"`
}

type HostInfo struct {
	GameID      int64  `json:"gameId"`
	Year        int    `json:"year"`
	City        string `json:"city"`
	CountryID   int64  `json:"countryId"`
	CountryName string `json:"countryName"`
}

type TopAthlete struct {
	AthleteID   int64  `json:"athleteId"`
	AthleteName string `json:"athleteName"`
	TotalMedals int    `json:"totalMedals"`
	HasRecord   bool   `json:"hasRecord"`
}

// RecordHolder is the case-2 / case-7 projection: an athlete that set an
// olympic record in a given event, flattened out of the embedded Records of an
// EventResult together with its event/discipline context.
type RecordHolder struct {
	AthleteID      int64   `json:"athleteId" bson:"athleteId"`
	AthleteName    string  `json:"athleteName" bson:"athleteName"`
	DisciplineID   int64   `json:"disciplineId" bson:"disciplineId"`
	DisciplineName string  `json:"disciplineName" bson:"disciplineName"`
	Sport          string  `json:"sport" bson:"sport"`
	EventID        int64   `json:"eventId" bson:"eventId"`
	GameName       string  `json:"gameName" bson:"gameName"`
	Type           string  `json:"record_type" bson:"record_type"`
	Metric         string  `json:"metric" bson:"metric"`
	Value          float64 `json:"value" bson:"value"`
}

// TeamGraph is the full denormalized neighbourhood of a team, used to mirror
// the whole relational structure around it into the derived stores. It carries
// the team's event, discipline, sport, game and country, plus every athlete on
// the roster (with their own home country), so a single fetch can drive the
// Redis leaderboards and the complete Neo4j projection.
type TeamGraph struct {
	TeamID         int64
	EventID        int64
	EventName      string
	DisciplineID   int64
	DisciplineName string
	SportID        int64
	SportName      string
	GameID         int64
	GameYear       int
	GameCity       string
	CountryID      int64
	CountryName    string
	Athletes       []GraphAthlete
}

// GraphAthlete is one roster member of a TeamGraph, with the home country it
// represents.
type GraphAthlete struct {
	ID          int64
	Name        string
	CountryID   int64
	CountryName string
}

// TeamListing is a team together with the country it represents, used by the
// console pickers (e.g. choosing a team within an event).
type TeamListing struct {
	TeamID      int64
	CountryID   int64
	CountryName string
}

// RealizeSummary is what the console reports after simulating an event: the
// fake results, medals and records that were fanned out across the stores.
type RealizeSummary struct {
	EventID        int64
	EventName      string
	DisciplineName string
	Sport          string
	Participants   int
	Medals         []string
	Records        int
	// WorldRecord is a human-readable note about the world record after the event:
	// whether it was broken (and by whom) or which past mark still stands. Empty
	// for events that do not produce records (e.g. tournaments).
	WorldRecord string
}
