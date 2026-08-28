package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type sequenceLotteryRandom struct {
	values []int
	calls  []int
}

type lotteryRepositoryStub struct {
	LotteryRepository
	participate      func(context.Context, int64, int64, string, time.Time, LotteryRandomSource) (*LotteryParticipationResult, error)
	listDueCampaigns func(context.Context, time.Time, int) ([]int64, error)
	drawScheduled    func(context.Context, int64, *int64, time.Time, LotteryRandomSource) (*LotteryDrawResult, error)
}

func (r *lotteryRepositoryStub) Participate(ctx context.Context, userID, campaignID int64, key string, now time.Time, random LotteryRandomSource) (*LotteryParticipationResult, error) {
	return r.participate(ctx, userID, campaignID, key, now, random)
}

func (r *lotteryRepositoryStub) ListDueScheduledCampaignIDs(ctx context.Context, now time.Time, limit int) ([]int64, error) {
	return r.listDueCampaigns(ctx, now, limit)
}

func (r *lotteryRepositoryStub) DrawScheduled(ctx context.Context, campaignID int64, triggeredBy *int64, now time.Time, random LotteryRandomSource) (*LotteryDrawResult, error) {
	return r.drawScheduled(ctx, campaignID, triggeredBy, now, random)
}

type lotteryAuthCacheInvalidatorStub struct {
	mu       sync.Mutex
	userIDs  []int64
	ctxError error
}

func (*lotteryAuthCacheInvalidatorStub) InvalidateAuthCacheByKey(context.Context, string)    {}
func (*lotteryAuthCacheInvalidatorStub) InvalidateAuthCacheByGroupID(context.Context, int64) {}
func (s *lotteryAuthCacheInvalidatorStub) InvalidateAuthCacheByUserID(ctx context.Context, userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userIDs = append(s.userIDs, userID)
	s.ctxError = ctx.Err()
}

func (r *sequenceLotteryRandom) Intn(max int) (int, error) {
	r.calls = append(r.calls, max)
	if len(r.values) == 0 {
		return 0, errors.New("lottery random sequence exhausted")
	}
	value := r.values[0]
	r.values = r.values[1:]
	if value < 0 || value >= max {
		return 0, errors.New("lottery random value out of range")
	}
	return value, nil
}

func (*sequenceLotteryRandom) RedeemCode() (string, error) {
	return "0123456789abcdef0123456789abcdef", nil
}

func validLotteryInput() LotteryCampaignInput {
	startsAt := time.Now().UTC().Add(time.Hour)
	return LotteryCampaignInput{
		Name:         "ModelPort launch draw",
		Mode:         LotteryModeInstant,
		Status:       LotteryCampaignDraft,
		StartsAt:     startsAt,
		EndsAt:       startsAt.Add(24 * time.Hour),
		PerUserLimit: 2,
		Prizes: []LotteryPrizeInput{
			{
				Name: "Balance reward", PrizeType: LotteryPrizeBalance, BalanceAmount: 10,
				ProbabilityBPS: 2500, Inventory: 20, IsEnabled: true,
			},
		},
	}
}

func TestNormalizeLotteryCampaignInput(t *testing.T) {
	input := validLotteryInput()
	input.RequiredSubscriptionGroupIDs = []int64{9, 3, 9}
	require.NoError(t, normalizeLotteryCampaignInput(&input))
	require.Equal(t, []int64{3, 9}, input.RequiredSubscriptionGroupIDs)
	require.Nil(t, input.DrawAt)
}

func TestNormalizeLotteryCampaignInputRejectsProbabilityOverflow(t *testing.T) {
	input := validLotteryInput()
	input.Prizes = append(input.Prizes, LotteryPrizeInput{
		Name: "Another reward", PrizeType: LotteryPrizeBalance, BalanceAmount: 1,
		ProbabilityBPS: 8000, Inventory: 1, IsEnabled: true,
	})
	err := normalizeLotteryCampaignInput(&input)
	require.Error(t, err)
	require.Contains(t, err.Error(), "10000")
}

func TestNormalizeLotteryCampaignInputRequiresScheduledDrawAfterEntryWindow(t *testing.T) {
	input := validLotteryInput()
	input.Mode = LotteryModeScheduled
	drawAt := input.EndsAt.Add(-time.Second)
	input.DrawAt = &drawAt
	require.ErrorIs(t, normalizeLotteryCampaignInput(&input), ErrLotteryInvalid)

	drawAt = input.EndsAt
	input.DrawAt = &drawAt
	require.NoError(t, normalizeLotteryCampaignInput(&input))
}

func TestNormalizeLotteryCampaignInputValidatesFullDrawParticipantLimit(t *testing.T) {
	input := validLotteryInput()
	input.Mode = LotteryModeScheduled
	drawAt := input.EndsAt
	input.DrawAt = &drawAt
	limit := 250
	input.FullDrawParticipantLimit = &limit
	require.NoError(t, normalizeLotteryCampaignInput(&input))
	require.Equal(t, 250, *input.FullDrawParticipantLimit)

	limit = 0
	require.ErrorIs(t, normalizeLotteryCampaignInput(&input), ErrLotteryInvalid)
	limit = 1000001
	require.ErrorIs(t, normalizeLotteryCampaignInput(&input), ErrLotteryInvalid)

	input = validLotteryInput()
	limit = 10
	input.FullDrawParticipantLimit = &limit
	require.NoError(t, normalizeLotteryCampaignInput(&input))
	require.Nil(t, input.FullDrawParticipantLimit)
}

func TestNormalizeLotteryCampaignInputValidatesPrizePayload(t *testing.T) {
	groupID := int64(7)
	tests := []struct {
		name    string
		prize   LotteryPrizeInput
		isValid bool
	}{
		{
			name: "subscription code",
			prize: LotteryPrizeInput{Name: "Pro access", PrizeType: LotteryPrizeSubscriptionCode,
				SubscriptionGroupID: &groupID, SubscriptionValidityDays: 30,
				ProbabilityBPS: 1000, Inventory: 3, IsEnabled: true},
			isValid: true,
		},
		{
			name: "subscription without group",
			prize: LotteryPrizeInput{Name: "Invalid", PrizeType: LotteryPrizeSubscriptionCode,
				SubscriptionValidityDays: 30, ProbabilityBPS: 1000, Inventory: 3, IsEnabled: true},
		},
		{
			name: "zero balance",
			prize: LotteryPrizeInput{Name: "Invalid", PrizeType: LotteryPrizeBalance,
				ProbabilityBPS: 1000, Inventory: 3, IsEnabled: true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validLotteryInput()
			input.Prizes = []LotteryPrizeInput{test.prize}
			err := normalizeLotteryCampaignInput(&input)
			if test.isValid {
				require.NoError(t, err)
			} else {
				require.True(t, errors.Is(err, ErrLotteryInvalid))
			}
		})
	}
}

func TestSelectLotteryPrizeUsesStableBasisPointIntervals(t *testing.T) {
	prizes := []LotteryPrize{
		{ID: 1, ProbabilityBPS: 1000, Inventory: 1, IsEnabled: true},
		{ID: 2, ProbabilityBPS: 2000, Inventory: 2, IsEnabled: true},
	}
	require.Equal(t, int64(1), SelectLotteryPrize(prizes, 0).ID)
	require.Equal(t, int64(1), SelectLotteryPrize(prizes, 999).ID)
	require.Equal(t, int64(2), SelectLotteryPrize(prizes, 1000).ID)
	require.Equal(t, int64(2), SelectLotteryPrize(prizes, 2999).ID)
	require.Nil(t, SelectLotteryPrize(prizes, 3000))
	require.Nil(t, SelectLotteryPrize(prizes, 10000))
}

func TestSelectLotteryPrizeDoesNotRedistributeExhaustedProbability(t *testing.T) {
	prizes := []LotteryPrize{
		{ID: 1, ProbabilityBPS: 1000, Inventory: 1, AwardedCount: 1, IsEnabled: true},
		{ID: 2, ProbabilityBPS: 2000, Inventory: 2, IsEnabled: true},
	}
	require.Nil(t, SelectLotteryPrize(prizes, 500))
	require.Equal(t, int64(2), SelectLotteryPrize(prizes, 1500).ID)
}

func TestAllocateScheduledLotteryPrizesExhaustsInventoryWhenParticipantsAreSufficient(t *testing.T) {
	prizes := []LotteryPrize{
		{ID: 1, ProbabilityBPS: 0, Inventory: 2, AwardedCount: 1, IsEnabled: true},
		{ID: 2, ProbabilityBPS: 0, Inventory: 2, IsEnabled: true},
		{ID: 3, ProbabilityBPS: 10000, Inventory: 9, IsEnabled: false},
		{ID: 4, ProbabilityBPS: 10000, Inventory: 1, AwardedCount: 1, IsEnabled: true},
	}
	random := &sequenceLotteryRandom{values: []int{
		9999, 9999, 9999, 9999, 9999,
		4, 2,
		0, 0,
		2, 0,
	}}

	assignments, err := AllocateScheduledLotteryPrizes(prizes, 5, random)

	require.NoError(t, err)
	require.Len(t, assignments, 5)
	winnerCounts := make(map[int64]int)
	for _, assignment := range assignments {
		if assignment != nil {
			winnerCounts[assignment.ID]++
		}
	}
	require.Equal(t, map[int64]int{1: 1, 2: 2}, winnerCounts)
	require.Equal(t, 2, prizes[0].AwardedCount)
	require.Equal(t, 2, prizes[1].AwardedCount)
	require.Zero(t, prizes[2].AwardedCount)
	require.Equal(t, 1, prizes[3].AwardedCount)
	require.Empty(t, random.values)
}

func TestAllocateScheduledLotteryPrizesDoesNotForceAwardsWhenParticipantsAreInsufficient(t *testing.T) {
	prizes := []LotteryPrize{
		{ID: 1, ProbabilityBPS: 0, Inventory: 3, IsEnabled: true},
	}
	random := &sequenceLotteryRandom{values: []int{9999, 9999}}

	assignments, err := AllocateScheduledLotteryPrizes(prizes, 2, random)

	require.NoError(t, err)
	require.Equal(t, []*LotteryPrize{nil, nil}, assignments)
	require.Zero(t, prizes[0].AwardedCount)
	require.Empty(t, random.values)
}

func TestLotteryServiceStopWithoutRepositoryDoesNotBlock(t *testing.T) {
	service := NewLotteryService(nil, nil, nil)
	done := make(chan struct{})
	go func() {
		service.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("LotteryService.Stop blocked without a repository")
	}
}

func TestLotterySchedulerGivesEachCampaignAnIndependentTimeout(t *testing.T) {
	var (
		mu       sync.Mutex
		calls    []int64
		ctxError error
	)
	repo := &lotteryRepositoryStub{
		listDueCampaigns: func(context.Context, time.Time, int) ([]int64, error) {
			return []int64{11, 12}, nil
		},
		drawScheduled: func(ctx context.Context, campaignID int64, _ *int64, _ time.Time, _ LotteryRandomSource) (*LotteryDrawResult, error) {
			mu.Lock()
			calls = append(calls, campaignID)
			if campaignID == 12 {
				ctxError = ctx.Err()
			}
			mu.Unlock()
			if campaignID == 11 {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return &LotteryDrawResult{CampaignID: campaignID}, nil
		},
	}
	svc := NewLotteryService(repo, nil, nil)

	svc.drawDueCampaignsWithTimeouts(time.Second, 10*time.Millisecond)

	require.Equal(t, []int64{11, 12}, calls)
	require.NoError(t, ctxError, "a timed-out campaign must not cancel later campaigns")
}

func TestLotteryBalanceRewardInvalidationDoesNotReuseCancelledRequestContext(t *testing.T) {
	repo := &lotteryRepositoryStub{
		participate: func(context.Context, int64, int64, string, time.Time, LotteryRandomSource) (*LotteryParticipationResult, error) {
			return &LotteryParticipationResult{Entry: &LotteryEntry{
				UserID: 42, Status: LotteryEntryWon, PrizeType: LotteryPrizeBalance,
			}}, nil
		},
	}
	invalidator := &lotteryAuthCacheInvalidatorStub{}
	svc := NewLotteryService(repo, nil, invalidator)
	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.Participate(requestCtx, 42, 7, "request-1")

	require.NoError(t, err)
	require.Equal(t, []int64{42}, invalidator.userIDs)
	require.NoError(t, invalidator.ctxError)
}
