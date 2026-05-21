package repository

import (
	"context"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"github.com/ObiaNzk/bdd-2-JOJO/internal/model"
)

type Neo4jRepository struct {
	driver neo4j.DriverWithContext
}

func NewNeo4jRepository(driver neo4j.DriverWithContext) *Neo4jRepository {
	return &Neo4jRepository{driver: driver}
}

func (r *Neo4jRepository) write(ctx context.Context, cypher string, params map[string]any) error {
	_, err := neo4j.ExecuteQuery(ctx, r.driver, cypher, params,
		neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	return err
}

// --- Entity upserts (mirror base entities into the graph as they are created) ---

func (r *Neo4jRepository) UpsertCountry(ctx context.Context, c *model.Country) error {
	return r.write(ctx, `MERGE (c:Country {id: $id}) SET c.name = $name`,
		map[string]any{"id": c.ID, "name": c.Name})
}

func (r *Neo4jRepository) UpsertOlympicGame(ctx context.Context, g *model.OlympicGame) error {
	return r.write(ctx, `MERGE (g:OlympicGame {id: $id}) SET g.year = $year, g.city = $city`,
		map[string]any{"id": g.ID, "year": g.Year, "city": g.City})
}

func (r *Neo4jRepository) UpsertSport(ctx context.Context, s *model.Sport) error {
	return r.write(ctx, `MERGE (s:Sport {id: $id}) SET s.name = $name`,
		map[string]any{"id": s.ID, "name": s.Name})
}

func (r *Neo4jRepository) UpsertDiscipline(ctx context.Context, d *model.Discipline) error {
	return r.write(ctx,
		`MERGE (d:Discipline {id: $id}) SET d.name = $name
		 MERGE (s:Sport {id: $sportId})
		 MERGE (d)-[:PART_OF]->(s)`,
		map[string]any{"id": d.ID, "name": d.Name, "sportId": d.SportID})
}

func (r *Neo4jRepository) UpsertAthlete(ctx context.Context, a *model.Athlete) error {
	return r.write(ctx,
		`MERGE (a:Athlete {id: $id}) SET a.name = $name
		 MERGE (c:Country {id: $countryId})
		 MERGE (a)-[:REPRESENTS]->(c)`,
		map[string]any{"id": a.ID, "name": a.Name, "countryId": a.CountryID})
}

func (r *Neo4jRepository) UpsertEvent(ctx context.Context, e *model.Event) error {
	return r.write(ctx,
		`MERGE (e:Event {id: $id}) SET e.name = $name
		 MERGE (d:Discipline {id: $disciplineId})
		 MERGE (g:OlympicGame {id: $gameId})
		 MERGE (e)-[:OF]->(d)
		 MERGE (e)-[:IN_GAME]->(g)`,
		map[string]any{"id": e.ID, "name": e.Name, "disciplineId": e.DisciplineID, "gameId": e.GameID})
}

// LinkCountryToGame records that a country takes part in an olympic game.
func (r *Neo4jRepository) LinkCountryToGame(ctx context.Context, gameID, countryID int64) error {
	return r.write(ctx,
		`MERGE (c:Country {id: $countryId})
		 MERGE (g:OlympicGame {id: $gameId})
		 MERGE (c)-[:PARTICIPATES_IN]->(g)`,
		map[string]any{"gameId": gameID, "countryId": countryID})
}

// --- Graph projection ---

// SyncTeamGraph mirrors the full relational neighbourhood of a team into the
// graph in a single statement: sport, discipline, game, event, country and team
// nodes with their structural relationships, plus every athlete on the roster
// (member of the team and representing its own country). It is idempotent (all
// MERGE) so the seed's repeated fan-out converges instead of duplicating.
func (r *Neo4jRepository) SyncTeamGraph(ctx context.Context, tg *model.TeamGraph) error {
	athletes := make([]map[string]any, 0, len(tg.Athletes))
	for _, a := range tg.Athletes {
		athletes = append(athletes, map[string]any{
			"id":          a.ID,
			"name":        a.Name,
			"countryId":   a.CountryID,
			"countryName": a.CountryName,
		})
	}
	return r.write(ctx,
		`MERGE (s:Sport {id: $sportId})           SET s.name = $sportName
		 MERGE (d:Discipline {id: $disciplineId}) SET d.name = $disciplineName
		 MERGE (d)-[:PART_OF]->(s)
		 MERGE (g:OlympicGame {id: $gameId})      SET g.year = $gameYear, g.city = $gameCity
		 MERGE (e:Event {id: $eventId})           SET e.name = $eventName
		 MERGE (e)-[:OF]->(d)
		 MERGE (e)-[:IN_GAME]->(g)
		 MERGE (tc:Country {id: $countryId})      SET tc.name = $countryName
		 MERGE (t:Team {id: $teamId})             SET t.name = 'team_' + toString($teamId)
		 MERGE (t)-[:COMPETED_IN]->(e)
		 MERGE (t)-[:REPRESENTS]->(tc)
		 MERGE (t)-[:IN_GAME]->(g)
		 WITH t
		 UNWIND $athletes AS ath
		   MERGE (a:Athlete {id: ath.id})         SET a.name = ath.name
		   MERGE (ac:Country {id: ath.countryId}) SET ac.name = ath.countryName
		   MERGE (a)-[:MEMBER_OF]->(t)
		   MERGE (a)-[:REPRESENTS]->(ac)`,
		map[string]any{
			"teamId":         tg.TeamID,
			"eventId":        tg.EventID,
			"eventName":      tg.EventName,
			"disciplineId":   tg.DisciplineID,
			"disciplineName": tg.DisciplineName,
			"sportId":        tg.SportID,
			"sportName":      tg.SportName,
			"gameId":         tg.GameID,
			"gameYear":       tg.GameYear,
			"gameCity":       tg.GameCity,
			"countryId":      tg.CountryID,
			"countryName":    tg.CountryName,
			"athletes":       athletes,
		})
}

// LinkTeamWonMedal records that a team won a medal. The team node is expected to
// already exist (SyncTeamGraph runs first in the same fan-out).
func (r *Neo4jRepository) LinkTeamWonMedal(ctx context.Context, teamID, medalID int64, medalType model.MedalType) error {
	return r.write(ctx,
		`MATCH (t:Team {id: $teamId})
		 MERGE (m:Medal {id: $medalId}) SET m.name = $type
		 MERGE (t)-[:WON]->(m)`,
		map[string]any{"teamId": teamID, "medalId": medalID, "type": string(medalType)})
}

// CountMedalsByCountryAndDiscipline resolves case 6 over the graph: a country's
// medals across every edition, broken down by discipline.
func (r *Neo4jRepository) CountMedalsByCountryAndDiscipline(ctx context.Context, countryID int64) ([]model.DisciplineMedalCount, error) {
	res, err := neo4j.ExecuteQuery(ctx, r.driver,
		`MATCH (c:Country {id: $countryId})<-[:REPRESENTS]-(t:Team)
		 MATCH (t)-[:WON]->(m:Medal)
		 MATCH (t)-[:COMPETED_IN]->(:Event)-[:OF]->(d:Discipline)
		 RETURN d.id AS disciplineId, d.name AS disciplineName,
		        count(CASE WHEN m.name = 'gold'   THEN 1 END) AS gold,
		        count(CASE WHEN m.name = 'silver' THEN 1 END) AS silver,
		        count(CASE WHEN m.name = 'bronze' THEN 1 END) AS bronze,
		        count(m) AS total
		 ORDER BY total DESC`,
		map[string]any{"countryId": countryID},
		neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		return nil, err
	}

	out := make([]model.DisciplineMedalCount, 0, len(res.Records))
	for _, rec := range res.Records {
		disciplineID, _, _ := neo4j.GetRecordValue[int64](rec, "disciplineId")
		disciplineName, _, _ := neo4j.GetRecordValue[string](rec, "disciplineName")
		gold, _, _ := neo4j.GetRecordValue[int64](rec, "gold")
		silver, _, _ := neo4j.GetRecordValue[int64](rec, "silver")
		bronze, _, _ := neo4j.GetRecordValue[int64](rec, "bronze")
		total, _, _ := neo4j.GetRecordValue[int64](rec, "total")
		out = append(out, model.DisciplineMedalCount{
			DisciplineID:   disciplineID,
			DisciplineName: disciplineName,
			Gold:           int(gold),
			Silver:         int(silver),
			Bronze:         int(bronze),
			Total:          int(total),
		})
	}
	return out, nil
}

// ListAthletesWithMedalsInMultipleDisciplines resolves case 4: athletes whose
// medal-winning teams span at least minDisciplines distinct disciplines.
func (r *Neo4jRepository) ListAthletesWithMedalsInMultipleDisciplines(ctx context.Context, minDisciplines int) ([]model.AthleteDisciplines, error) {
	res, err := neo4j.ExecuteQuery(ctx, r.driver,
		`MATCH (a:Athlete)-[:MEMBER_OF]->(t:Team)-[:WON]->(:Medal)
		 MATCH (t)-[:COMPETED_IN]->(:Event)-[:OF]->(d:Discipline)
		 WITH a, count(DISTINCT d) AS disciplineCount, collect(DISTINCT d.name) AS disciplines
		 WHERE disciplineCount >= $min
		 RETURN a.id AS athleteId, a.name AS athleteName, disciplineCount, disciplines
		 ORDER BY disciplineCount DESC`,
		map[string]any{"min": minDisciplines},
		neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		return nil, err
	}

	out := make([]model.AthleteDisciplines, 0, len(res.Records))
	for _, rec := range res.Records {
		athleteID, _, _ := neo4j.GetRecordValue[int64](rec, "athleteId")
		athleteName, _, _ := neo4j.GetRecordValue[string](rec, "athleteName")
		count, _, _ := neo4j.GetRecordValue[int64](rec, "disciplineCount")

		var disciplines []string
		if raw, ok := rec.Get("disciplines"); ok {
			if list, ok := raw.([]any); ok {
				for _, v := range list {
					if s, ok := v.(string); ok {
						disciplines = append(disciplines, s)
					}
				}
			}
		}

		out = append(out, model.AthleteDisciplines{
			AthleteID:       athleteID,
			AthleteName:     athleteName,
			DisciplineCount: int(count),
			Disciplines:     disciplines,
		})
	}
	return out, nil
}
