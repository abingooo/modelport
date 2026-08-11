package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClaimGrokVoiceAccountBindingWithTTLUsesOneAtomicClaim(t *testing.T) {
	cache := &grokVoiceBindingCacheStub{bindings: make(map[string]grokVoiceBinding)}
	svc := &OpenAIGatewayService{cache: cache}
	groupID := int64(7)
	const ttl = 24 * time.Hour

	err := svc.claimGrokVoiceAccountBindingWithTTL(
		context.Background(), &groupID, "voice-binding", 30, "realtime conversation", ttl,
	)
	require.NoError(t, err)
	require.Equal(t, 1, cache.claimCalls)
	require.Zero(t, cache.setCalls)
	require.Equal(t, ttl, cache.ttl)
}
