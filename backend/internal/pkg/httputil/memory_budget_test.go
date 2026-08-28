package httputil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRequestBodyMemoryBudgetWaitsUntilLeaseRelease(t *testing.T) {
	budget := NewRequestBodyMemoryBudget(2)
	first, err := budget.Acquire(context.Background(), 1)
	require.NoError(t, err)
	second, err := budget.Acquire(context.Background(), 1)
	require.NoError(t, err)

	waitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = budget.Acquire(waitCtx, 1)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	first.Release()
	replacement, err := budget.Acquire(context.Background(), 1)
	require.NoError(t, err)
	replacement.Release()
	second.Release()
}
