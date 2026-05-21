package repository

import (
	"context"
	"fmt"

	"github.com/ObiaNzk/bdd-2-JOJO/internal/model"
)

// DefaultCountries is the starter set ensured on startup (Argentina included).
var DefaultCountries = []string{"Argentina", "Brasil", "Estados Unidos", "Francia", "Japón"}

// DefaultAthletesPerCountry is how many athletes each default country is topped
// up to, enough to fill a football roster (11) without creating more on the fly.
const DefaultAthletesPerCountry = 11

// EnsureDefaults seeds the starter set of countries and athletes so events can be
// populated right away. It is idempotent: countries are matched by name and each
// country is topped up to DefaultAthletesPerCountry athletes. Returns how many
// countries and athletes were newly created.
func (r *PostgresRepository) EnsureDefaults(ctx context.Context) (countriesCreated, athletesCreated int, err error) {
	existing, err := r.ListCountries(ctx)
	if err != nil {
		return 0, 0, err
	}
	byName := make(map[string]int64, len(existing))
	for _, c := range existing {
		byName[c.Name] = c.ID
	}

	for _, name := range DefaultCountries {
		id, ok := byName[name]
		if !ok {
			country := &model.Country{Name: name}
			if err := r.CreateCountry(ctx, country); err != nil {
				return countriesCreated, athletesCreated, err
			}
			id = country.ID
			countriesCreated++
		}
		added, err := r.EnsureCountryAthletes(ctx, id, name, DefaultAthletesPerCountry)
		if err != nil {
			return countriesCreated, athletesCreated, err
		}
		athletesCreated += added
	}
	return countriesCreated, athletesCreated, nil
}

// EnsureCountryAthletes makes sure a country has at least `need` athletes,
// creating generic ones for the missing slots. Returns how many it created.
func (r *PostgresRepository) EnsureCountryAthletes(ctx context.Context, countryID int64, countryName string, need int) (int, error) {
	athletes, err := r.ListAthletesByCountry(ctx, countryID)
	if err != nil {
		return 0, err
	}
	created := 0
	for n := len(athletes) + 1; len(athletes)+created < need; n++ {
		a := &model.Athlete{CountryID: countryID, Name: fmt.Sprintf("Deportista %s %d", countryName, n)}
		if err := r.CreateAthlete(ctx, a); err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}
