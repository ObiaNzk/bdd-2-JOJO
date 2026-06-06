package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ObiaNzk/bdd-2-JOJO/internal/model"
	"github.com/ObiaNzk/bdd-2-JOJO/internal/repository"
	"github.com/ObiaNzk/bdd-2-JOJO/internal/service"
)

// Console is the interactive front-end. It uses the Postgres repository
// directly for plain setup CRUD/listings and the service for the orchestrated
// "realize event" fan-out and the read use cases.
type Console struct {
	db  *repository.PostgresRepository
	svc *service.Service
	in  *bufio.Scanner
}

func newConsole(db *repository.PostgresRepository, svc *service.Service) *Console {
	return &Console{db: db, svc: svc, in: bufio.NewScanner(os.Stdin)}
}

func (c *Console) Run(ctx context.Context) error {
	for {
		printMenu()
		switch c.readLine("Opción> ") {
		case "1":
			c.createCountry(ctx)
		case "2":
			c.createGame(ctx)
		case "3":
			c.createSport(ctx)
		case "4":
			c.createDiscipline(ctx)
		case "5":
			c.createAthlete(ctx)
		case "6":
			c.registerCountryInGame(ctx)
		case "7":
			c.createEvent(ctx)
		case "8":
			c.createTeam(ctx)
		case "9":
			c.addAthleteToTeam(ctx)
		case "10":
			c.realizeEvent(ctx)
		case "11":
			c.queries(ctx)
			continue // the queries submenu handles its own pauses
		case "12":
			c.loadDefaults(ctx)
		case "13":
			c.generateTeamsForEvent(ctx)
		case "0", "q", "salir", "exit":
			fmt.Println("chau")
			return nil
		case "":
			continue // ignore empty input, no pause
		default:
			fmt.Println("  ! opción desconocida")
		}
		c.pause()
	}
}

func printMenu() {
	fmt.Print(`
========================================
 Consola del Servicio Olímpico
========================================
 Carga (Postgres):
   1) Crear país
   2) Crear juego olímpico
   3) Crear deporte
   4) Crear disciplina
   5) Crear atleta
   6) Registrar país(es) en juego (uno o 'todos')
   7) Crear evento (si la disciplina es de torneo, crea solo la semifinal)
   8) Crear equipo
   9) Agregar atletas a equipo (uno o varios, separados por coma)
 Ejecutar:
  10) Realizar evento (resultados, medallas y records fake -> Mongo/Redis/Neo4j)
 Consultas:
  11) Casos de uso (consultas del enunciado)
 Generadores:
  12) Cargar datos por defecto (países + deportistas, incluye Argentina)
  13) Generar equipos para un evento (elegís juego y evento; crea los que necesita con plantel)
   0) Salir
`)
}

func (c *Console) queries(ctx context.Context) {
	for {
		fmt.Print(`
 Casos de uso (entre corchetes la base de datos de la que sale):
   1) Medallero por juego                        [Redis]
   2) Récords olímpicos                          [MongoDB]
   3) Eventos más populares (por países)         [PostgreSQL]
   4) Atletas con medallas en varias disciplinas [Neo4j]
   5) Sedes (país anfitrión por edición)         [PostgreSQL]
   6) Medallas de un país por disciplina         [Neo4j]
   7) Top atletas (>= N medallas o récord)       [Redis + MongoDB]
   0) Volver
`)
		switch c.readLine("Consulta> ") {
		case "1":
			c.medalRanking(ctx)
		case "2":
			c.recordHolders(ctx)
		case "3":
			c.popularEvents(ctx)
		case "4":
			c.multiDiscipline(ctx)
		case "5":
			c.hosts(ctx)
		case "6":
			c.medalsByCountryDiscipline(ctx)
		case "7":
			c.topAthletes(ctx)
		case "0", "":
			return
		default:
			fmt.Println("  ! opción desconocida")
		}
		c.pause()
	}
}

// --- Setup actions ---

func (c *Console) createCountry(ctx context.Context) {
	country := &model.Country{Name: c.readLine("Nombre: ")}
	if err := c.db.CreateCountry(ctx, country); err != nil {
		c.fail(err)
		return
	}
	if err := c.svc.SyncCountry(ctx, country); err != nil {
		c.fail(err)
		return
	}
	fmt.Printf("  ok: país #%d creado\n", country.ID)
}

func (c *Console) createGame(ctx context.Context) {
	countries, err := c.db.ListCountries(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	if !c.require(len(countries), "creá un país (opción 1) para la sede") {
		return
	}
	year, ok := c.askInt("Año: ")
	if !ok {
		return
	}
	city := c.readLine("Ciudad: ")
	c.printCountries(ctx)
	host, ok := c.askInt("ID país sede: ")
	if !ok {
		return
	}
	game := &model.OlympicGame{Year: int(year), City: city, HostCountryID: host}
	if err := c.db.CreateOlympicGame(ctx, game); err != nil {
		c.fail(err)
		return
	}
	if err := c.svc.SyncOlympicGame(ctx, game); err != nil {
		c.fail(err)
		return
	}
	fmt.Printf("  ok: juego #%d creado\n", game.ID)
}

func (c *Console) createSport(ctx context.Context) {
	sport := &model.Sport{Name: c.readLine("Nombre: ")}
	if err := c.db.CreateSport(ctx, sport); err != nil {
		c.fail(err)
		return
	}
	if err := c.svc.SyncSport(ctx, sport); err != nil {
		c.fail(err)
		return
	}
	fmt.Printf("  ok: deporte #%d creado\n", sport.ID)
}

func (c *Console) createDiscipline(ctx context.Context) {
	sports, err := c.db.ListSports(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	if !c.require(len(sports), "creá un deporte (opción 3)") {
		return
	}
	c.printSports(ctx)
	sportID, ok := c.askInt("ID deporte: ")
	if !ok {
		return
	}
	disc := &model.Discipline{SportID: sportID, Name: c.readLine("Nombre: ")}
	if err := c.db.CreateDiscipline(ctx, disc); err != nil {
		c.fail(err)
		return
	}
	if err := c.svc.SyncDiscipline(ctx, disc); err != nil {
		c.fail(err)
		return
	}
	fmt.Printf("  ok: disciplina #%d creada\n", disc.ID)
}

func (c *Console) createAthlete(ctx context.Context) {
	countries, err := c.db.ListCountries(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	if !c.require(len(countries), "creá un país (opción 1)") {
		return
	}
	c.printCountries(ctx)
	countryID, ok := c.askInt("ID país: ")
	if !ok {
		return
	}
	name := c.readLine("Nombre: ")
	ath := &model.Athlete{CountryID: countryID, Name: name}
	if err := c.db.CreateAthlete(ctx, ath); err != nil {
		c.fail(err)
		return
	}
	if err := c.svc.SyncAthlete(ctx, ath); err != nil {
		c.fail(err)
		return
	}
	fmt.Printf("  ok: atleta #%d creado\n", ath.ID)
}

func (c *Console) registerCountryInGame(ctx context.Context) {
	games, err := c.db.ListOlympicGames(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	if !c.require(len(games), "creá un juego olímpico (opción 2)") {
		return
	}
	countries, err := c.db.ListCountries(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	if !c.require(len(countries), "creá un país (opción 1)") {
		return
	}
	c.printGames(ctx)
	gameID, ok := c.askInt("ID juego: ")
	if !ok {
		return
	}
	c.printCountries(ctx)
	sel := c.readLine("ID país (o 'todos' para registrar todos): ")

	if strings.EqualFold(sel, "todos") || sel == "*" {
		registered := 0
		for _, co := range countries {
			if _, err := c.db.GetGameCountryID(ctx, gameID, co.ID); err == nil {
				continue // ya estaba registrado
			}
			gc := &model.GameCountry{GameID: gameID, CountryID: co.ID}
			if err := c.db.RegisterCountryInGame(ctx, gc); err != nil {
				c.fail(err)
				continue
			}
			if err := c.svc.SyncCountryInGame(ctx, gameID, co.ID); err != nil {
				c.fail(err)
				continue
			}
			registered++
		}
		fmt.Printf("  ok: %d país(es) registrado(s) en el juego #%d\n", registered, gameID)
		return
	}

	countryID, err := strconv.ParseInt(sel, 10, 64)
	if err != nil {
		fmt.Println("  ! ingresá un número de país o 'todos'")
		return
	}
	gc := &model.GameCountry{GameID: gameID, CountryID: countryID}
	if err := c.db.RegisterCountryInGame(ctx, gc); err != nil {
		c.fail(err)
		return
	}
	if err := c.svc.SyncCountryInGame(ctx, gameID, countryID); err != nil {
		c.fail(err)
		return
	}
	fmt.Printf("  ok: país registrado en juego (game_country #%d)\n", gc.ID)
}

func (c *Console) createEvent(ctx context.Context) {
	games, err := c.db.ListOlympicGames(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	if !c.require(len(games), "creá un juego olímpico (opción 2)") {
		return
	}
	disciplines, err := c.db.ListDisciplines(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	if !c.require(len(disciplines), "creá una disciplina (opción 4)") {
		return
	}
	c.printGames(ctx)
	gameID, ok := c.askInt("ID juego: ")
	if !ok {
		return
	}
	c.printDisciplines(ctx)
	discID, ok := c.askInt("ID disciplina: ")
	if !ok {
		return
	}
	name := c.readLine("Nombre: ")
	date := c.askDate("Fecha (AAAA-MM-DD, vacío=hoy): ")

	// Tournament disciplines are not a single event: creating one lays down only
	// the opening round (the semifinal). The final and the bronze match are born
	// when the semifinal is realized. Everything else is one plain event.
	if service.ResultFormat(disciplineName(disciplines, discID)) == "tournament" {
		c.createTournamentSemifinal(ctx, gameID, discID, name, date)
		return
	}

	ev := &model.Event{GameID: gameID, DisciplineID: discID, Name: name, Date: date}
	if err := c.db.CreateEvent(ctx, ev); err != nil {
		c.fail(err)
		return
	}
	if err := c.svc.SyncEvent(ctx, ev); err != nil {
		c.fail(err)
		return
	}
	fmt.Printf("  ok: evento #%d creado\n", ev.ID)
}

// createTournamentSemifinal lays down only the opening round of a knockout
// tournament: an empty semifinal. The final and the bronze match are created
// when the semifinal is realized — that is when the teams that reach them are
// known — so they do not exist (and cannot be picked) until then.
func (c *Console) createTournamentSemifinal(ctx context.Context, gameID, discID int64, name string, base time.Time) {
	semi := &model.Event{GameID: gameID, DisciplineID: discID, Name: "Semifinal " + name, Date: base, Phase: "semifinal"}
	if err := c.createTournamentEvent(ctx, semi); err != nil {
		return
	}

	fmt.Printf("  ok: torneo creado -> semifinal #%d\n", semi.ID)
	fmt.Printf("  siguiente: asigná 4 equipos a la semifinal #%d (opción 8 o 13) y realizala (opción 10).\n", semi.ID)
	fmt.Println("            al realizar la semifinal se crean la final y el tercer puesto con sus equipos.")
}

// disciplineName resolves a discipline id to its name within an already-loaded
// slice (empty string if not found).
func disciplineName(disciplines []model.Discipline, id int64) string {
	for _, d := range disciplines {
		if d.ID == id {
			return d.Name
		}
	}
	return ""
}

// createTournamentEvent persists one tournament event and mirrors it to Neo4j,
// reporting any failure to the console.
func (c *Console) createTournamentEvent(ctx context.Context, ev *model.Event) error {
	if err := c.db.CreateEvent(ctx, ev); err != nil {
		c.fail(err)
		return err
	}
	if err := c.svc.SyncEvent(ctx, ev); err != nil {
		c.fail(err)
		return err
	}
	return nil
}

func (c *Console) createTeam(ctx context.Context) {
	events, err := c.db.ListEvents(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	if !c.require(len(events), "creá un evento (opción 7)") {
		return
	}
	countries, err := c.db.ListCountries(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	if !c.require(len(countries), "creá un país (opción 1)") {
		return
	}
	c.printEvents(ctx)
	eventID, ok := c.askInt("ID evento: ")
	if !ok {
		return
	}
	ev, err := c.db.GetEventByID(ctx, eventID)
	if err != nil {
		c.fail(err)
		return
	}
	if ev.PreviousEventID != nil {
		fmt.Println("  ! ese evento recibe sus equipos al realizar la ronda anterior (semifinal); no se le asignan a mano")
		return
	}
	c.printCountries(ctx)
	countryID, ok := c.askInt("ID país: ")
	if !ok {
		return
	}
	gcID, err := c.db.GetGameCountryID(ctx, ev.GameID, countryID)
	if err != nil {
		fmt.Println("  ! el país no está registrado en el juego de este evento (usá la opción 6 primero)")
		return
	}
	team := &model.Team{GameCountryID: gcID, EventID: eventID}
	if err := c.db.CreateTeam(ctx, team); err != nil {
		c.fail(err)
		return
	}
	if err := c.svc.SyncTeam(ctx, team.ID); err != nil {
		c.fail(err)
		return
	}
	fmt.Printf("  ok: equipo #%d creado\n", team.ID)
}

func (c *Console) addAthleteToTeam(ctx context.Context) {
	events, err := c.db.ListEvents(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	if !c.require(len(events), "creá un evento (opción 7) con equipos") {
		return
	}
	c.printEvents(ctx)
	eventID, ok := c.askInt("ID evento: ")
	if !ok {
		return
	}

	teams, err := c.db.ListTeamsByEventWithCountry(ctx, eventID)
	if err != nil {
		c.fail(err)
		return
	}
	if len(teams) == 0 {
		fmt.Println("  (este evento no tiene equipos)")
		return
	}
	fmt.Printf("  equipos en evento #%d:\n", eventID)
	for _, t := range teams {
		fmt.Printf("    #%d %s\n", t.TeamID, t.CountryName)
	}
	teamID, ok := c.askInt("ID equipo: ")
	if !ok {
		return
	}

	var team model.TeamListing
	found := false
	for _, t := range teams {
		if t.TeamID == teamID {
			team = t
			found = true
			break
		}
	}
	if !found {
		fmt.Println("  ! ese equipo no pertenece al evento")
		return
	}

	// Only athletes from the team's country can join it.
	athletes, err := c.db.ListAthletesByCountry(ctx, team.CountryID)
	if err != nil {
		c.fail(err)
		return
	}
	if len(athletes) == 0 {
		fmt.Printf("  (no hay atletas de %s; creá uno con la opción 5)\n", team.CountryName)
		return
	}
	fmt.Printf("  atletas de %s:\n", team.CountryName)
	valid := make(map[int64]string, len(athletes))
	for _, a := range athletes {
		fmt.Printf("    #%d %s\n", a.ID, a.Name)
		valid[a.ID] = a.Name
	}

	added := 0
	for _, tok := range strings.Split(c.readLine("ID atleta(s), separados por coma: "), ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		id, err := strconv.ParseInt(tok, 10, 64)
		if err != nil {
			fmt.Printf("  ! '%s' no es un número, lo salteo\n", tok)
			continue
		}
		name, ok := valid[id]
		if !ok {
			fmt.Printf("  ! #%d no es atleta de %s, lo salteo\n", id, team.CountryName)
			continue
		}
		if err := c.db.AddAthleteToTeam(ctx, teamID, id); err != nil {
			c.fail(err)
			continue
		}
		fmt.Printf("  ok: %s (#%d) agregado\n", name, id)
		added++
	}
	if added > 0 {
		if err := c.svc.SyncTeam(ctx, teamID); err != nil {
			c.fail(err)
			return
		}
	}
	fmt.Printf("  %d atleta(s) agregado(s) al equipo #%d\n", added, teamID)
}

// --- Realize ---

func (c *Console) realizeEvent(ctx context.Context) {
	events, err := c.db.ListEvents(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	if !c.require(len(events), "creá un evento (opción 7) y generale equipos (opción 13)") {
		return
	}
	// An event is realizable only once its previous round is realized: the final
	// and the bronze match get their teams from the semifinal, so they cannot run
	// (and must not appear) until it is realized.
	realized := make(map[int64]bool, len(events))
	for _, r := range events {
		realized[r.ID] = r.Realized
	}
	ready := make([]model.Event, 0, len(events))
	for _, r := range events {
		if r.Realized {
			continue
		}
		if r.PreviousEventID == nil || realized[*r.PreviousEventID] {
			ready = append(ready, r)
		}
	}
	if !c.require(len(ready), "no hay eventos listos para realizar (realizá primero la semifinal, o creá uno nuevo con la opción 7)") {
		return
	}
	fmt.Println("  eventos listos para realizar:")
	readyByID := make(map[int64]bool, len(ready))
	for _, r := range ready {
		readyByID[r.ID] = true
		fmt.Printf("    #%d %s (juego #%d, disciplina #%d)\n", r.ID, r.Name, r.GameID, r.DisciplineID)
	}
	eventID, ok := c.askInt("ID evento a realizar: ")
	if !ok {
		return
	}
	if !readyByID[eventID] {
		fmt.Println("  ! ese evento no está listo: o ya fue realizado, o depende de una ronda previa que todavía no realizaste")
		return
	}
	summary, err := c.svc.RealizeEvent(ctx, eventID)
	if err != nil {
		c.fail(err)
		return
	}
	fmt.Printf("  ok: evento %q realizado (%s / %s)\n",
		summary.EventName, summary.DisciplineName, summary.Sport)
	fmt.Printf("       participantes: %d, records: %d\n", summary.Participants, summary.Records)
	for _, m := range summary.Medals {
		fmt.Printf("       medalla: %s\n", m)
	}
}

// --- Generators ---

// loadDefaults seeds the starter set of countries and athletes (idempotent).
// The same set is also ensured automatically on startup; this re-runs it on
// demand, e.g. after truncating data mid-session.
func (c *Console) loadDefaults(ctx context.Context) {
	newCountries, newAthletes, err := c.db.EnsureDefaults(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	if err := c.svc.SyncBaseEntities(ctx); err != nil {
		c.fail(err)
		return
	}
	fmt.Printf("  ok: datos por defecto cargados (%d países nuevos, %d deportistas nuevos)\n", newCountries, newAthletes)
}

// generateTeamsForEvent picks a game and one of its events, then creates exactly
// the teams the event needs (4 for a tournament semifinal, teamsPerEvent
// otherwise) without asking — one per country, filled with the discipline's
// roster size (11 for football, 1 otherwise). Countries are auto-registered in
// the game and athletes are created on the fly when a country is short.
func (c *Console) generateTeamsForEvent(ctx context.Context) {
	games, err := c.db.ListOlympicGames(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	if !c.require(len(games), "creá un juego olímpico (opción 2)") {
		return
	}
	c.printGames(ctx)
	gameID, ok := c.askInt("ID juego: ")
	if !ok {
		return
	}

	events, err := c.db.ListEventsByGame(ctx, gameID)
	if err != nil {
		c.fail(err)
		return
	}
	if len(events) == 0 {
		fmt.Println("  (este juego no tiene eventos; creá uno con la opción 7)")
		return
	}
	// Only root events take teams directly (the final and the bronze match get
	// theirs when the semifinal is realized, so they carry a previous_event_id and
	// are skipped). Events that are already realized, or that already hold their
	// full complement of teams, are left out too — there is nothing to add to them.
	fmt.Printf("  eventos del juego #%d con cupo para equipos:\n", gameID)
	selectable := make(map[int64]bool)
	for _, e := range events {
		if e.PreviousEventID != nil || e.Realized {
			continue
		}
		dn, err := c.disciplineName(ctx, e.DisciplineID)
		if err != nil {
			c.fail(err)
			return
		}
		teams, err := c.db.ListTeamsByEventWithCountry(ctx, e.ID)
		if err != nil {
			c.fail(err)
			return
		}
		target := eventTeamTarget(dn)
		if len(teams) >= target {
			continue // ya tiene todos sus equipos
		}
		fmt.Printf("    #%d %s (disciplina #%d) — %d/%d equipos\n", e.ID, e.Name, e.DisciplineID, len(teams), target)
		selectable[e.ID] = true
	}
	if len(selectable) == 0 {
		fmt.Println("  (no hay eventos con cupo: ya tienen sus equipos o ya fueron realizados; creá uno con la opción 7)")
		return
	}
	eventID, ok := c.askInt("ID evento: ")
	if !ok {
		return
	}
	if !selectable[eventID] {
		fmt.Println("  ! ese evento no está en la lista (no es de este juego, ya está completo/realizado, o recibe equipos de una ronda previa)")
		return
	}
	event, err := c.db.GetEventByID(ctx, eventID)
	if err != nil {
		c.fail(err)
		return
	}

	discName, err := c.disciplineName(ctx, event.DisciplineID)
	if err != nil {
		c.fail(err)
		return
	}
	roster := rosterSizeForDiscipline(discName)
	targetTeams := eventTeamTarget(discName)

	teamed := map[int64]bool{}
	existingTeams, err := c.db.ListTeamsByEventWithCountry(ctx, eventID)
	if err != nil {
		c.fail(err)
		return
	}
	for _, t := range existingTeams {
		teamed[t.CountryID] = true
	}

	slots := targetTeams - len(existingTeams)
	if slots <= 0 {
		fmt.Printf("  (el evento ya tiene %d equipo(s); necesita %d)\n", len(existingTeams), targetTeams)
		return
	}

	countries, err := c.db.ListCountries(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	if !c.require(len(countries), "creá países (opción 1) o cargá los datos por defecto (opción 12)") {
		return
	}
	candidates := make([]model.Country, 0, len(countries))
	for _, co := range countries {
		if !teamed[co.ID] {
			candidates = append(candidates, co)
		}
	}
	if len(candidates) == 0 {
		fmt.Println("  (todos los países ya tienen equipo en este evento; cargá más con la opción 1 o 12)")
		return
	}

	// Generate the missing teams, capped by how many countries are still free.
	count := slots
	if count > len(candidates) {
		count = len(candidates)
	}
	fmt.Printf("  disciplina %q -> %d deportista(s) por equipo; genero %d equipo(s) (objetivo %d)\n",
		discName, roster, count, targetTeams)

	generated := 0
	for i := 0; i < count; i++ {
		co := candidates[i]
		gcID, err := c.ensureGameCountry(ctx, gameID, co.ID)
		if err != nil {
			c.fail(err)
			continue
		}
		team := &model.Team{GameCountryID: gcID, EventID: eventID}
		if err := c.db.CreateTeam(ctx, team); err != nil {
			c.fail(err)
			continue
		}
		if _, err := c.db.EnsureCountryAthletes(ctx, co.ID, co.Name, roster); err != nil {
			c.fail(err)
			continue
		}
		athletes, err := c.db.ListAthletesByCountry(ctx, co.ID)
		if err != nil {
			c.fail(err)
			continue
		}
		added := 0
		for _, a := range athletes[:roster] {
			if err := c.db.AddAthleteToTeam(ctx, team.ID, a.ID); err != nil {
				c.fail(err)
				continue
			}
			added++
		}
		if err := c.svc.SyncTeam(ctx, team.ID); err != nil {
			c.fail(err)
			continue
		}
		fmt.Printf("  ok: equipo #%d (%s) con %d deportista(s)\n", team.ID, co.Name, added)
		generated++
	}
	total := len(existingTeams) + generated
	fmt.Printf("  %d equipo(s) generado(s) en el evento #%d (total: %d)\n", generated, eventID, total)
	switch {
	case total < targetTeams:
		fmt.Printf("  -> faltan %d equipo(s) para los %d que necesita el evento (cargá más países con la opción 1 o 12)\n", targetTeams-total, targetTeams)
	case service.ResultFormat(discName) == "tournament":
		fmt.Println("  -> 4 equipos: realizá la semifinal (opción 10) y se asignan solos a la final y al tercer puesto")
	default:
		fmt.Println("  -> listo para 'Realizar evento' (opción 10)")
	}
}

// ensureGameCountry returns the game_country id for (game, country), registering
// the country in the game first if needed.
func (c *Console) ensureGameCountry(ctx context.Context, gameID, countryID int64) (int64, error) {
	if gcID, err := c.db.GetGameCountryID(ctx, gameID, countryID); err == nil {
		return gcID, nil
	}
	gc := &model.GameCountry{GameID: gameID, CountryID: countryID}
	if err := c.db.RegisterCountryInGame(ctx, gc); err != nil {
		return 0, err
	}
	return c.db.GetGameCountryID(ctx, gameID, countryID)
}

func (c *Console) disciplineName(ctx context.Context, id int64) (string, error) {
	discs, err := c.db.ListDisciplines(ctx)
	if err != nil {
		return "", err
	}
	for _, d := range discs {
		if d.ID == id {
			return d.Name, nil
		}
	}
	return "", fmt.Errorf("disciplina #%d no encontrada", id)
}

// teamsPerEvent is the unified number of teams generated for a single (non
// tournament) event. Tournaments override it with exactly 4 for their semifinal.
const teamsPerEvent = 8

// eventTeamTarget is how many teams an event needs: 4 for a tournament semifinal,
// teamsPerEvent for any single event.
func eventTeamTarget(discName string) int {
	if service.ResultFormat(discName) == "tournament" {
		return 4
	}
	return teamsPerEvent
}

// rosterSizeForDiscipline maps a discipline to how many athletes a team needs:
// 11 for football, 1 for the individual disciplines.
func rosterSizeForDiscipline(name string) int {
	d := strings.ToLower(name)
	if strings.Contains(d, "fútbol") || strings.Contains(d, "futbol") || strings.Contains(d, "football") {
		return 11
	}
	return 1
}

// --- Queries ---

// explain prints, for a use case, the database it reads from and the query it
// runs there. It mirrors what the repository layer actually executes.
func (c *Console) explain(db, query string) {
	fmt.Printf("  base de datos: %s\n", db)
	fmt.Println("  query:")
	for _, line := range strings.Split(query, "\n") {
		fmt.Printf("    %s\n", line)
	}
	fmt.Println()
}

func (c *Console) medalRanking(ctx context.Context) {
	c.explain("Redis", `ZREVRANGE medals:country:{gameID} 0 {N-1} WITHSCORES   -- ranking + total
HGETALL  medals:country:{gameID}:gold                  -- desglose por tipo
HGETALL  medals:country:{gameID}:silver
HGETALL  medals:country:{gameID}:bronze`)
	c.printGames(ctx)
	gameID, ok := c.askInt("ID juego: ")
	if !ok {
		return
	}
	rows, err := c.svc.MedalRanking(ctx, gameID, 20)
	if err != nil {
		c.fail(err)
		return
	}
	if len(rows) == 0 {
		fmt.Println("  (este juego no tiene medallas todavía)")
		return
	}
	for _, r := range rows {
		fmt.Printf("  %-20s O:%d P:%d B:%d (total %d)\n", r.CountryName, r.Gold, r.Silver, r.Bronze, r.Total)
	}
}

func (c *Console) multiDiscipline(ctx context.Context) {
	c.explain("Neo4j", `MATCH (a:Athlete)-[:MEMBER_OF]->(t:Team)-[:WON]->(:Medal)
MATCH (t)-[:COMPETED_IN]->(:Event)-[:OF]->(d:Discipline)
WITH a, count(DISTINCT d) AS disciplineCount, collect(DISTINCT d.name) AS disciplines
WHERE disciplineCount >= $min
RETURN a.id, a.name, disciplineCount, disciplines
ORDER BY disciplineCount DESC`)
	min, ok := c.askInt("Mín. disciplinas (ej: 2): ")
	if !ok {
		return
	}
	rows, err := c.svc.AthletesInMultipleDisciplines(ctx, int(min))
	if err != nil {
		c.fail(err)
		return
	}
	for _, r := range rows {
		fmt.Printf("  %-20s %d disciplinas: %s\n", r.AthleteName, r.DisciplineCount, strings.Join(r.Disciplines, ", "))
	}
}

func (c *Console) recordHolders(ctx context.Context) {
	c.explain("MongoDB", `db.event_results.aggregate([
  { $unwind: "$records" },
  { $project: { athleteId: "$records.athleteId", athleteName: "$records.athleteName",
                disciplineId: 1, disciplineName: 1, sport: 1, eventId: 1, gameName: 1,
                record_type: "$records.record_type", metric: "$records.metric", value: "$records.value" } }
])`)
	rows, err := c.svc.RecordHolders(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	for _, r := range rows {
		fmt.Printf("  %-20s %s %s=%.0f (%s) @ %s\n", r.AthleteName, r.DisciplineName, r.Metric, r.Value, r.Type, r.GameName)
	}
}

func (c *Console) hosts(ctx context.Context) {
	c.explain("PostgreSQL", `SELECT g.id, g.year, g.city, c.id, c.name
FROM olympic_games g
JOIN countries c ON c.id = g.host_country_id
ORDER BY g.year`)
	rows, err := c.svc.Hosts(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	for _, r := range rows {
		fmt.Printf("  %d %s -> %s\n", r.Year, r.City, r.CountryName)
	}
}

func (c *Console) popularEvents(ctx context.Context) {
	c.explain("PostgreSQL", `SELECT e.id, e.name, COUNT(DISTINCT gc.country_id) AS countries
FROM events e
JOIN teams t           ON t.event_id = e.id
JOIN game_countries gc ON gc.id = t.game_country_id
WHERE e.game_id = $1
GROUP BY e.id, e.name
ORDER BY countries DESC
LIMIT $2`)
	c.printGames(ctx)
	gameID, ok := c.askInt("ID juego: ")
	if !ok {
		return
	}
	rows, err := c.svc.PopularEvents(ctx, gameID, 20)
	if err != nil {
		c.fail(err)
		return
	}
	for _, r := range rows {
		fmt.Printf("  %-28s %d países\n", r.EventName, r.CountriesCount)
	}
}

func (c *Console) medalsByCountryDiscipline(ctx context.Context) {
	c.explain("Neo4j", `MATCH (c:Country {id:$countryId})<-[:REPRESENTS]-(t:Team)
MATCH (t)-[:WON]->(m:Medal)
MATCH (t)-[:COMPETED_IN]->(:Event)-[:OF]->(d:Discipline)
RETURN d.id, d.name,
       count(CASE WHEN m.name='gold'   THEN 1 END) AS gold,
       count(CASE WHEN m.name='silver' THEN 1 END) AS silver,
       count(CASE WHEN m.name='bronze' THEN 1 END) AS bronze,
       count(m) AS total
ORDER BY total DESC`)
	c.printCountries(ctx)
	countryID, ok := c.askInt("ID país: ")
	if !ok {
		return
	}
	rows, err := c.svc.MedalsByCountryAndDiscipline(ctx, countryID)
	if err != nil {
		c.fail(err)
		return
	}
	for _, r := range rows {
		fmt.Printf("  %-20s O:%d P:%d B:%d (total %d)\n", r.DisciplineName, r.Gold, r.Silver, r.Bronze, r.Total)
	}
}

func (c *Console) topAthletes(ctx context.Context) {
	c.explain("Redis + MongoDB", `Redis:   ZUNION numkeys medals:athlete:{g1} medals:athlete:{g2} ... AGGREGATE SUM WITHSCORES
         -- unifica los leaderboards per-juego en totales históricos
MongoDB: db.event_results.aggregate([{ $unwind: "$records" },
           { $project: { athleteName: "$records.athleteName" } }])
         -- quién tiene récord (cualquier edición)
(se combinan: aparece quien acumula >= N medallas O tiene récord)`)
	min, ok := c.askInt("Mín. medallas (ej: 3): ")
	if !ok {
		return
	}
	rows, err := c.svc.TopAthletes(ctx, int(min))
	if err != nil {
		c.fail(err)
		return
	}
	for _, r := range rows {
		rec := ""
		if r.HasRecord {
			rec = " (récord)"
		}
		fmt.Printf("  %-20s %d medallas%s\n", r.AthleteName, r.TotalMedals, rec)
	}
}

// --- Listing printers (help pick foreign keys) ---

func (c *Console) printCountries(ctx context.Context) {
	rows, err := c.db.ListCountries(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	fmt.Println("  países:")
	for _, r := range rows {
		fmt.Printf("    #%d %s\n", r.ID, r.Name)
	}
}

func (c *Console) printGames(ctx context.Context) {
	rows, err := c.db.ListOlympicGames(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	fmt.Println("  juegos:")
	for _, r := range rows {
		fmt.Printf("    #%d %s %d\n", r.ID, r.City, r.Year)
	}
}

func (c *Console) printSports(ctx context.Context) {
	rows, err := c.db.ListSports(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	fmt.Println("  deportes:")
	for _, r := range rows {
		fmt.Printf("    #%d %s\n", r.ID, r.Name)
	}
}

func (c *Console) printDisciplines(ctx context.Context) {
	rows, err := c.db.ListDisciplines(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	fmt.Println("  disciplinas:")
	for _, r := range rows {
		fmt.Printf("    #%d %s (deporte #%d)\n", r.ID, r.Name, r.SportID)
	}
}

func (c *Console) printEvents(ctx context.Context) {
	rows, err := c.db.ListEvents(ctx)
	if err != nil {
		c.fail(err)
		return
	}
	fmt.Println("  eventos:")
	for _, r := range rows {
		estado := ""
		if r.Realized {
			estado = " [realizado]"
		}
		fmt.Printf("    #%d %s (juego #%d, disciplina #%d)%s\n", r.ID, r.Name, r.GameID, r.DisciplineID, estado)
	}
}

// --- Input helpers ---

func (c *Console) readLine(prompt string) string {
	fmt.Print(prompt)
	if !c.in.Scan() {
		return "0" // EOF -> behave like "exit"
	}
	return strings.TrimSpace(c.in.Text())
}

// pause waits for Enter so errors and results stay on screen before the menu is
// drawn again.
func (c *Console) pause() {
	fmt.Print("\n  (enter para volver al menú) ")
	c.in.Scan()
}

func (c *Console) askInt(prompt string) (int64, bool) {
	s := c.readLine(prompt)
	if s == "" {
		fmt.Println("  ! se requiere un número")
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		fmt.Println("  ! número inválido")
		return 0, false
	}
	return n, true
}

func (c *Console) askDate(prompt string) time.Time {
	s := c.readLine(prompt)
	if s == "" {
		return time.Now()
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		fmt.Println("  ! fecha inválida, uso hoy")
		return time.Now()
	}
	return t
}

// require prints a hint and returns false when a prerequisite list is empty, so
// the caller aborts before prompting for data it cannot satisfy.
func (c *Console) require(n int, hint string) bool {
	if n == 0 {
		fmt.Printf("  ! primero %s\n", hint)
		return false
	}
	return true
}

func (c *Console) fail(err error) {
	fmt.Printf("  ! error: %s\n", humanize(err))
}

// humanize turns the technical errors bubbling up from the service and the
// stores into short Spanish messages for the console user.
func humanize(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "no rows in result set"):
		return "no se encontró el registro solicitado"
	case strings.Contains(msg, "already realized"):
		return "el evento ya fue realizado; no se puede realizar de nuevo"
	case strings.Contains(msg, "at least 3 teams"):
		return "el evento necesita al menos 3 equipos"
	case strings.Contains(msg, "foreign key constraint"):
		return "referencia inválida: el registro relacionado no existe"
	case strings.Contains(msg, "duplicate key"), strings.Contains(msg, "unique constraint"):
		return "ya existe un registro con esos datos"
	default:
		return msg
	}
}
