package httputil

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ErrRequestBodyMemoryBudgetExceeded reports a reservation larger than the
// configured process-local capacity.
var ErrRequestBodyMemoryBudgetExceeded = errors.New("request body memory reservation exceeds budget capacity")

// RequestBodyMemoryBudget bounds aggregate in-flight request-body working sets.
type RequestBodyMemoryBudget struct {
	mu       sync.Mutex
	capacity int64
	used     int64
	changed  chan struct{}
}

// RequestBodyMemoryLease keeps a reservation until all body consumers finish.
type RequestBodyMemoryLease struct {
	once   sync.Once
	budget *RequestBodyMemoryBudget
	weight int64
}

func NewRequestBodyMemoryBudget(capacity int64) *RequestBodyMemoryBudget {
	if capacity <= 0 {
		return nil
	}
	return &RequestBodyMemoryBudget{capacity: capacity, changed: make(chan struct{})}
}

func (budget *RequestBodyMemoryBudget) Capacity() int64 {
	if budget == nil {
		return 0
	}
	budget.mu.Lock()
	defer budget.mu.Unlock()
	return budget.capacity
}

func (budget *RequestBodyMemoryBudget) SetCapacity(capacity int64) {
	if budget == nil || capacity <= 0 {
		return
	}
	budget.mu.Lock()
	if budget.capacity != capacity {
		budget.capacity = capacity
		budget.notifyWaitersLocked()
	}
	budget.mu.Unlock()
}

func (budget *RequestBodyMemoryBudget) Acquire(ctx context.Context, weight int64) (*RequestBodyMemoryLease, error) {
	if budget == nil || weight <= 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		budget.mu.Lock()
		capacity := budget.capacity
		if weight > capacity {
			budget.mu.Unlock()
			return nil, fmt.Errorf("%w: requested=%d capacity=%d", ErrRequestBodyMemoryBudgetExceeded, weight, capacity)
		}
		if budget.used <= capacity-weight {
			budget.used += weight
			budget.mu.Unlock()
			return &RequestBodyMemoryLease{budget: budget, weight: weight}, nil
		}
		changed := budget.changed
		budget.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

func (lease *RequestBodyMemoryLease) Release() {
	if lease == nil {
		return
	}
	lease.once.Do(func() {
		if lease.budget == nil || lease.weight <= 0 {
			return
		}
		lease.budget.mu.Lock()
		lease.budget.used -= lease.weight
		if lease.budget.used < 0 {
			lease.budget.used = 0
		}
		lease.budget.notifyWaitersLocked()
		lease.budget.mu.Unlock()
	})
}

func (budget *RequestBodyMemoryBudget) notifyWaitersLocked() {
	close(budget.changed)
	budget.changed = make(chan struct{})
}

// RequestBodyWorkingSetBytes computes a conservative multi-buffer reservation.
func RequestBodyWorkingSetBytes(maxBytes int64, simultaneousBuffers int64) (int64, error) {
	if maxBytes <= 0 || simultaneousBuffers <= 0 {
		return 0, nil
	}
	if maxBytes > (int64(^uint64(0)>>1) / simultaneousBuffers) {
		return 0, errors.New("request body working-set estimate overflows int64")
	}
	return maxBytes * simultaneousBuffers, nil
}
