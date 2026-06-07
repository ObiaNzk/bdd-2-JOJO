package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

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

// GetAthleteMedalsAcrossGames merges every per-game athlete leaderboard with
// ZUNION (SUM) so case 7 sees totals across the whole olympic history. Returns
// a name -> total medal count map; an empty list of games yields an empty map.
func (r *RedisRepository) GetAthleteMedalsAcrossGames(ctx context.Context, gameIDs []int64) (map[string]int, error) {
	if len(gameIDs) == 0 {
		return map[string]int{}, nil
	}
	keys := make([]string, len(gameIDs))
	for i, id := range gameIDs {
		keys[i] = athleteMedalsKey(id)
	}
	zs, err := r.rdb.ZUnionWithScores(ctx, redis.ZStore{Keys: keys, Aggregate: "SUM"}).Result()
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(zs))
	for _, z := range zs {
		out[z.Member.(string)] = int(z.Score)
	}
	return out, nil
}

// AddCountryToEvent records a participating country for popularity counting (case 3).
func (r *RedisRepository) AddCountryToEvent(ctx context.Context, eventID, countryID int64) error {
	return r.rdb.PFAdd(ctx, eventCountriesKey(eventID), strconv.FormatInt(countryID, 10)).Err()
}

// GetEventPopularity returns the approximate distinct-country count for an event.
func (r *RedisRepository) GetEventPopularity(ctx context.Context, eventID int64) (int64, error) {
	return r.rdb.PFCount(ctx, eventCountriesKey(eventID)).Result()
}

// --- Use-case read-through cache ---
//
// Every use-case query is cached here as a JSON blob under a "usecase:" key with
// a short TTL: the first call runs the real query and stores its result; calls
// within the TTL are served straight from Redis without touching the underlying
// store. Generic on the result type so any use case can reuse it.

// CacheGetJSON loads a cached value into dest. It reports whether the key was
// present (a miss is not an error). A decode/Redis failure degrades to a miss so
// callers fall back to running the real query.
func (r *RedisRepository) CacheGetJSON(ctx context.Context, key string, dest any) (bool, error) {
	b, err := r.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(b, dest); err != nil {
		return false, err
	}
	return true, nil
}

// CacheSetJSON stores val as JSON under key, expiring after ttl.
func (r *RedisRepository) CacheSetJSON(ctx context.Context, key string, val any, ttl time.Duration) error {
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return r.rdb.Set(ctx, key, b, ttl).Err()
}
