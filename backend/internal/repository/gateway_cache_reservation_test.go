package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newGatewayCacheReservationTest(t *testing.T) (*gatewayCache, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return &gatewayCache{rdb: client}, server
}

func TestGatewayCacheGrokVoiceReservationCompetitionAndExpiry(t *testing.T) {
	cache, server := newGatewayCacheReservationTest(t)
	ctx := context.Background()
	const (
		groupID    int64 = 41
		libraryKey       = "openai:grok-voice-library:user-1"
	)

	type reservationResult struct {
		reserved bool
		err      error
	}
	const competitors = 16
	start := make(chan struct{})
	results := make(chan reservationResult, competitors)
	for i := 0; i < competitors; i++ {
		go func(i int) {
			<-start
			reserved, err := cache.ReserveGrokVoiceLibrary(
				ctx, groupID, libraryKey, int64(100+i), "token-"+string(rune('a'+i)), time.Minute,
			)
			results <- reservationResult{reserved: reserved, err: err}
		}(i)
	}
	close(start)
	winners := 0
	for i := 0; i < competitors; i++ {
		result := <-results
		require.NoError(t, result.err)
		if result.reserved {
			winners++
		}
	}
	require.Equal(t, 1, winners, "only one concurrent creator may hold the library reservation")

	server.FastForward(time.Minute + time.Second)
	reserved, err := cache.ReserveGrokVoiceLibrary(ctx, groupID, libraryKey, 202, "token-after-expiry", time.Minute)
	require.NoError(t, err)
	require.True(t, reserved, "an expired reservation must be reclaimable")
}

func TestGatewayCacheGrokVoiceReservationCommit(t *testing.T) {
	cache, _ := newGatewayCacheReservationTest(t)
	ctx := context.Background()
	const (
		groupID     int64 = 42
		accountID   int64 = 303
		libraryKey        = "openai:grok-voice-library:user-2"
		resourceKey       = "openai:grok-voice-resource:user-2:abcd1234"
	)

	reserved, err := cache.ReserveGrokVoiceLibrary(ctx, groupID, libraryKey, accountID, "commit-token", time.Minute)
	require.NoError(t, err)
	require.True(t, reserved)
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	require.NoError(t, cache.CommitGrokVoiceLibraryReservation(
		canceledCtx, groupID, libraryKey, resourceKey, accountID, "commit-token",
	))

	libraryAccount, err := cache.GetSessionAccountID(ctx, groupID, libraryKey)
	require.NoError(t, err)
	require.Equal(t, accountID, libraryAccount)
	resourceAccount, err := cache.GetSessionAccountID(ctx, groupID, resourceKey)
	require.NoError(t, err)
	require.Equal(t, accountID, resourceAccount)
	ttl, err := cache.rdb.TTL(ctx, buildSessionKey(groupID, libraryKey)).Result()
	require.NoError(t, err)
	require.Equal(t, time.Duration(-1), ttl, "committed ownership is permanent")
	require.Equal(t, int64(0), cache.rdb.Exists(ctx, buildSessionKey(groupID, libraryKey+":pending")).Val())

	err = cache.CommitGrokVoiceLibraryReservation(ctx, groupID, libraryKey, resourceKey, accountID, "commit-token")
	require.ErrorContains(t, err, "expired")
}

func TestGatewayCacheGrokVoiceReservationConflictsDoNotOverwriteBindings(t *testing.T) {
	cache, _ := newGatewayCacheReservationTest(t)
	ctx := context.Background()
	const (
		groupID     int64 = 43
		libraryKey        = "openai:grok-voice-library:user-3"
		resourceKey       = "openai:grok-voice-resource:user-3:abcd1234"
	)

	require.NoError(t, cache.SetSessionAccountID(ctx, groupID, libraryKey, 404, 0))
	reserved, err := cache.ReserveGrokVoiceLibrary(ctx, groupID, libraryKey, 505, "wrong-owner", time.Minute)
	require.ErrorContains(t, err, "library account conflict")
	require.False(t, reserved)
	require.Equal(t, int64(0), cache.rdb.Exists(ctx, buildSessionKey(groupID, libraryKey+":pending")).Val())

	reserved, err = cache.ReserveGrokVoiceLibrary(ctx, groupID, libraryKey, 404, "resource-conflict", time.Minute)
	require.NoError(t, err)
	require.True(t, reserved)
	require.NoError(t, cache.SetSessionAccountID(ctx, groupID, resourceKey, 606, 0))
	err = cache.CommitGrokVoiceLibraryReservation(ctx, groupID, libraryKey, resourceKey, 404, "resource-conflict")
	require.ErrorContains(t, err, "resource account conflict")

	libraryAccount, err := cache.GetSessionAccountID(ctx, groupID, libraryKey)
	require.NoError(t, err)
	require.Equal(t, int64(404), libraryAccount)
	resourceAccount, err := cache.GetSessionAccountID(ctx, groupID, resourceKey)
	require.NoError(t, err)
	require.Equal(t, int64(606), resourceAccount)
}

func TestGatewayCacheGrokVoiceReservationReleaseIsTokenSafeAfterExpiryAndCancellation(t *testing.T) {
	cache, server := newGatewayCacheReservationTest(t)
	ctx := context.Background()
	const (
		groupID    int64 = 44
		libraryKey       = "openai:grok-voice-library:user-4"
	)

	reserved, err := cache.ReserveGrokVoiceLibrary(ctx, groupID, libraryKey, 707, "old-token", time.Second)
	require.NoError(t, err)
	require.True(t, reserved)
	server.FastForward(2 * time.Second)
	reserved, err = cache.ReserveGrokVoiceLibrary(ctx, groupID, libraryKey, 707, "new-token", time.Minute)
	require.NoError(t, err)
	require.True(t, reserved)

	require.NoError(t, cache.ReleaseGrokVoiceLibraryReservation(ctx, groupID, libraryKey, 707, "old-token"))
	reserved, err = cache.ReserveGrokVoiceLibrary(ctx, groupID, libraryKey, 808, "competitor", time.Minute)
	require.NoError(t, err)
	require.False(t, reserved, "an expired owner's token must not release the new reservation")

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	require.NoError(t, cache.ReleaseGrokVoiceLibraryReservation(canceledCtx, groupID, libraryKey, 707, "new-token"))
	reserved, err = cache.ReserveGrokVoiceLibrary(ctx, groupID, libraryKey, 808, "competitor", time.Minute)
	require.NoError(t, err)
	require.True(t, reserved, "cleanup must complete even after the request context is canceled")
}
