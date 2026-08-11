package repository

import (
	"context"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newGatewayCacheClaimTest(t *testing.T) (*gatewayCache, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return &gatewayCache{rdb: client}, server
}

func TestGatewayCacheClaimSessionAccountIDWithTTLCreatesExpiringBinding(t *testing.T) {
	cache, server := newGatewayCacheClaimTest(t)
	ctx := context.Background()
	const (
		groupID     int64 = 101
		accountID   int64 = 201
		sessionHash       = "expiring"
	)

	boundAccountID, err := cache.ClaimSessionAccountIDWithTTL(ctx, groupID, sessionHash, accountID, 2*time.Minute)
	require.NoError(t, err)
	require.Equal(t, accountID, boundAccountID)
	require.Equal(t, 2*time.Minute, server.TTL(buildSessionKey(groupID, sessionHash)))

	server.FastForward(2*time.Minute + time.Millisecond)
	_, err = cache.GetSessionAccountID(ctx, groupID, sessionHash)
	require.Error(t, err)
}

func TestGatewayCacheClaimSessionAccountIDWithTTLRefreshesSameOwner(t *testing.T) {
	cache, server := newGatewayCacheClaimTest(t)
	ctx := context.Background()
	const (
		groupID     int64 = 102
		accountID   int64 = 202
		sessionHash       = "refresh"
	)

	_, err := cache.ClaimSessionAccountIDWithTTL(ctx, groupID, sessionHash, accountID, time.Minute)
	require.NoError(t, err)
	server.FastForward(30 * time.Second)

	boundAccountID, err := cache.ClaimSessionAccountIDWithTTL(ctx, groupID, sessionHash, accountID, 3*time.Minute)
	require.NoError(t, err)
	require.Equal(t, accountID, boundAccountID)
	require.Equal(t, 3*time.Minute, server.TTL(buildSessionKey(groupID, sessionHash)))
}

func TestGatewayCacheClaimSessionAccountIDWithTTLConflictPreservesOwnerAndTTL(t *testing.T) {
	cache, server := newGatewayCacheClaimTest(t)
	ctx := context.Background()
	const (
		groupID      int64 = 103
		ownerID      int64 = 203
		competitorID int64 = 204
		sessionHash        = "conflict"
	)

	_, err := cache.ClaimSessionAccountIDWithTTL(ctx, groupID, sessionHash, ownerID, 5*time.Minute)
	require.NoError(t, err)
	server.FastForward(time.Minute)
	key := buildSessionKey(groupID, sessionHash)
	ttlBefore := server.TTL(key)

	boundAccountID, err := cache.ClaimSessionAccountIDWithTTL(ctx, groupID, sessionHash, competitorID, 30*time.Minute)
	require.NoError(t, err)
	require.Equal(t, ownerID, boundAccountID)
	storedOwner, err := server.Get(key)
	require.NoError(t, err)
	require.Equal(t, strconv.FormatInt(ownerID, 10), storedOwner)
	require.Equal(t, ttlBefore, server.TTL(key))

	boundAccountID, err = cache.ClaimSessionAccountIDWithTTL(ctx, groupID, sessionHash, competitorID, 0)
	require.NoError(t, err)
	require.Equal(t, ownerID, boundAccountID)
	require.Equal(t, ttlBefore, server.TTL(key))
}

func TestGatewayCacheClaimSessionAccountIDWithTTLZeroIsPermanent(t *testing.T) {
	cache, server := newGatewayCacheClaimTest(t)
	ctx := context.Background()
	const (
		groupID   int64 = 104
		accountID int64 = 205
	)

	boundAccountID, err := cache.ClaimSessionAccountIDWithTTL(ctx, groupID, "new-permanent", accountID, 0)
	require.NoError(t, err)
	require.Equal(t, accountID, boundAccountID)
	require.Zero(t, server.TTL(buildSessionKey(groupID, "new-permanent")))

	_, err = cache.ClaimSessionAccountIDWithTTL(ctx, groupID, "persist-existing", accountID, time.Minute)
	require.NoError(t, err)
	boundAccountID, err = cache.ClaimSessionAccountID(ctx, groupID, "persist-existing", accountID)
	require.NoError(t, err)
	require.Equal(t, accountID, boundAccountID)
	require.Zero(t, server.TTL(buildSessionKey(groupID, "persist-existing")))
}

func TestGatewayCacheClaimSessionAccountIDWithTTLPreservesLargeInt64(t *testing.T) {
	cache, server := newGatewayCacheClaimTest(t)
	ctx := context.Background()
	const (
		groupID     int64 = 105
		accountID   int64 = math.MaxInt64
		sessionHash       = "large-account-id"
	)

	boundAccountID, err := cache.ClaimSessionAccountIDWithTTL(ctx, groupID, sessionHash, accountID, time.Minute)
	require.NoError(t, err)
	require.Equal(t, accountID, boundAccountID)
	storedOwner, err := server.Get(buildSessionKey(groupID, sessionHash))
	require.NoError(t, err)
	require.Equal(t, strconv.FormatInt(accountID, 10), storedOwner)

	boundAccountID, err = cache.ClaimSessionAccountIDWithTTL(ctx, groupID, sessionHash, accountID-1, 0)
	require.NoError(t, err)
	require.Equal(t, accountID, boundAccountID)
}
