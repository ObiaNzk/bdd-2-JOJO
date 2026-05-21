package repository

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"

	"github.com/ObiaNzk/bdd-2-JOJO/internal/model"
)

type RedisRepository struct {
	rdb *redis.Client
}

func NewRedisRepository(rdb *redis.Client) *RedisRepository {
	return &RedisRepository{rdb: rdb}
}

func medalRankingKey(gameID int64) string {
	return fmt.Sprintf("medals:country:%d", gameID)
}

func countryMedalsByTypeKey(gameID int64, medalType model.MedalType) string {
	return fmt.Sprintf("medals:country:%d:%s", gameID, medalType)
}

func athleteMedalsKey(gameID int64) string {
	return fmt.Sprintf("medals:athlete:%d", gameID)
}

func eventCountriesKey(eventID int64) string {
	return fmt.Sprintf("event:%d:countries", eventID)
}

// IncrCountryMedal bumps a country's total in the per-game leaderboard sorted
// set (used for ranking order) and its per-type tally in a hash (used for the
// gold/silver/bronze breakdown).
func (r *RedisRepository) IncrCountryMedal(ctx context.Context, gameID int64, countryName string, medalType model.MedalType) error {
	if err := r.rdb.ZIncrBy(ctx, medalRankingKey(gameID), 1, countryName).Err(); err != nil {
		return err
	}
	return r.rdb.HIncrBy(ctx, countryMedalsByTypeKey(gameID, medalType), countryName, 1).Err()
}

// IncrAthleteMedal bumps an athlete's medal count in the per-game leaderboard.
func (r *RedisRepository) IncrAthleteMedal(ctx context.Context, gameID int64, athleteName string) error {
	return r.rdb.ZIncrBy(ctx, athleteMedalsKey(gameID), 1, athleteName).Err()
}

// GetMedalRanking returns the top-N countries by medal count (case 1).
func (r *RedisRepository) GetMedalRanking(ctx context.Context, gameID int64, limit int) ([]model.MedalCount, error) {
	stop := int64(limit - 1)
	if limit <= 0 {
		stop = -1
	}
	zs, err := r.rdb.ZRevRangeWithScores(ctx, medalRankingKey(gameID), 0, stop).Result()
	if err != nil {
		return nil, err
	}

	gold, err := r.rdb.HGetAll(ctx, countryMedalsByTypeKey(gameID, model.Gold)).Result()
	if err != nil {
		return nil, err
	}
	silver, err := r.rdb.HGetAll(ctx, countryMedalsByTypeKey(gameID, model.Silver)).Result()
	if err != nil {
		return nil, err
	}
	bronze, err := r.rdb.HGetAll(ctx, countryMedalsByTypeKey(gameID, model.Bronze)).Result()
	if err != nil {
		return nil, err
	}

	out := make([]model.MedalCount, 0, len(zs))
	for _, z := range zs {
		name := z.Member.(string)
		out = append(out, model.MedalCount{
			CountryName: name,
			Gold:        atoiSafe(gold[name]),
			Silver:      atoiSafe(silver[name]),
			Bronze:      atoiSafe(bronze[name]),
			Total:       int(z.Score),
		})
	}
	return out, nil
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// GetAthletesWithMinMedals returns athletes with at least min medals and their
// totals (case 7, part A).
func (r *RedisRepository) GetAthletesWithMinMedals(ctx context.Context, gameID int64, min int) ([]model.TopAthlete, error) {
	zs, err := r.rdb.ZRangeByScoreWithScores(ctx, athleteMedalsKey(gameID), &redis.ZRangeBy{
		Min: strconv.Itoa(min),
		Max: "+inf",
	}).Result()
	if err != nil {
		return nil, err
	}

	out := make([]model.TopAthlete, 0, len(zs))
	for _, z := range zs {
		out = append(out, model.TopAthlete{AthleteName: z.Member.(string), TotalMedals: int(z.Score)})
	}
	return out, nil
}

// GetAthleteMedalCount returns an athlete's medal total (0 if absent).
func (r *RedisRepository) GetAthleteMedalCount(ctx context.Context, gameID int64, athleteName string) (int, error) {
	score, err := r.rdb.ZScore(ctx, athleteMedalsKey(gameID), athleteName).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return int(score), nil
}

// AddCountryToEvent records a participating country for popularity counting (case 3).
func (r *RedisRepository) AddCountryToEvent(ctx context.Context, eventID, countryID int64) error {
	return r.rdb.PFAdd(ctx, eventCountriesKey(eventID), strconv.FormatInt(countryID, 10)).Err()
}

// GetEventPopularity returns the approximate distinct-country count for an event.
func (r *RedisRepository) GetEventPopularity(ctx context.Context, eventID int64) (int64, error) {
	return r.rdb.PFCount(ctx, eventCountriesKey(eventID)).Result()
}
