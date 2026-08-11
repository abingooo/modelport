package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type grokVoiceReservationTTLCache struct {
	GatewayCache
	ttl time.Duration
}

func (c *grokVoiceReservationTTLCache) ReserveGrokVoiceLibrary(
	_ context.Context,
	_ int64,
	_ string,
	_ int64,
	_ string,
	ttl time.Duration,
) (bool, error) {
	c.ttl = ttl
	return true, nil
}

func (c *grokVoiceReservationTTLCache) CommitGrokVoiceLibraryReservation(
	context.Context, int64, string, string, int64, string,
) error {
	return nil
}

func (c *grokVoiceReservationTTLCache) ReleaseGrokVoiceLibraryReservation(
	context.Context, int64, string, int64, string,
) error {
	return nil
}

func TestReserveGrokCustomVoiceLibraryAccountUsesBoundedLongRequestTTL(t *testing.T) {
	cache := &grokVoiceReservationTTLCache{}
	svc := &OpenAIGatewayService{cache: cache}
	groupID := int64(45)

	token, err := svc.ReserveGrokCustomVoiceLibraryAccount(context.Background(), &groupID, 901, 902)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.Equal(t, 10*time.Minute, cache.ttl)
	require.Equal(t, grokVoiceLibraryReservationTTL, cache.ttl)
}
