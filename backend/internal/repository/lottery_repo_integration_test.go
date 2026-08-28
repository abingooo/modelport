//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type fixedLotteryRandom struct {
	roll int
}

func (r fixedLotteryRandom) Intn(int) (int, error) { return r.roll, nil }
func (fixedLotteryRandom) RedeemCode() (string, error) {
	return "0123456789abcdef0123456789abcdef", nil
}

func TestLotteryRepositoryConcurrentParticipationRespectsInventoryAndIdempotency(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	var userID int64
	err := integrationDB.QueryRowContext(ctx, `INSERT INTO users (email,password_hash,role,status,balance)
VALUES ($1,'test','user','active',0) RETURNING id`, fmt.Sprintf("lottery-%d@example.com", suffix)).Scan(&userID)
	require.NoError(t, err)

	repo := NewLotteryRepository(integrationDB)
	startsAt := time.Now().UTC().Add(-time.Minute)
	campaign, err := repo.Create(ctx, userID, service.LotteryCampaignInput{
		Name: "Concurrent inventory test", Mode: service.LotteryModeInstant,
		Status: service.LotteryCampaignActive, StartsAt: startsAt,
		EndsAt: startsAt.Add(time.Hour), PerUserLimit: 20,
		Prizes: []service.LotteryPrizeInput{{
			Name: "Only reward", PrizeType: service.LotteryPrizeBalance,
			BalanceAmount: 10, ProbabilityBPS: 10000, Inventory: 1, IsEnabled: true,
		}},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_events WHERE campaign_id=$1`, campaign.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_entries WHERE campaign_id=$1`, campaign.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_draw_runs WHERE campaign_id=$1`, campaign.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_campaigns WHERE id=$1`, campaign.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})

	const participants = 12
	entries := make(chan *service.LotteryEntry, participants)
	errorsCh := make(chan error, participants)
	var waitGroup sync.WaitGroup
	for index := 0; index < participants; index++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			result, participateErr := repo.Participate(ctx, userID, campaign.ID,
				fmt.Sprintf("request-%d", index), time.Now().UTC(), fixedLotteryRandom{roll: 0})
			if participateErr != nil {
				errorsCh <- participateErr
				return
			}
			entries <- result.Entry
		}(index)
	}
	waitGroup.Wait()
	close(entries)
	close(errorsCh)
	for participateErr := range errorsCh {
		require.NoError(t, participateErr)
	}

	winners := 0
	for entry := range entries {
		if entry.Status == service.LotteryEntryWon {
			winners++
		}
	}
	require.Equal(t, 1, winners)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT balance FROM users WHERE id=$1`, userID).Scan(&balance))
	require.Equal(t, 10.0, balance)

	firstResult, err := repo.Participate(ctx, userID, campaign.ID, "request-0", time.Now().UTC(), fixedLotteryRandom{roll: 0})
	require.NoError(t, err)
	require.True(t, firstResult.Replayed)
	secondResult, err := repo.Participate(ctx, userID, campaign.ID, "request-0", time.Now().UTC(), fixedLotteryRandom{roll: 9999})
	require.NoError(t, err)
	require.True(t, secondResult.Replayed)
	require.Equal(t, firstResult.Entry.ID, secondResult.Entry.ID)

	var entryCount, awardedCount, balanceEvents int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM lottery_entries WHERE campaign_id=$1`, campaign.ID).Scan(&entryCount))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT awarded_count FROM lottery_prizes WHERE campaign_id=$1`, campaign.ID).Scan(&awardedCount))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM lottery_events WHERE campaign_id=$1 AND event_type='balance_credited'`, campaign.ID).Scan(&balanceEvents))
	require.Equal(t, participants, entryCount)
	require.Equal(t, 1, awardedCount)
	require.Equal(t, 1, balanceEvents)
}

func TestLotteryRepositoryScheduledDrawIssuesRedeemCodeExactlyOnce(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	var userID, groupID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO users (email,password_hash,role,status,balance)
VALUES ($1,'test','user','active',0) RETURNING id`, fmt.Sprintf("lottery-scheduled-%d@example.com", suffix)).Scan(&userID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO groups (name,status,subscription_type)
VALUES ($1,'active','subscription') RETURNING id`, fmt.Sprintf("lottery-subscription-%d", suffix)).Scan(&groupID))

	repo := NewLotteryRepository(integrationDB)
	startsAt := time.Now().UTC().Add(-time.Minute)
	endsAt := startsAt.Add(2 * time.Minute)
	drawAt := endsAt
	campaign, err := repo.Create(ctx, userID, service.LotteryCampaignInput{
		Name: "Scheduled subscription reward", Mode: service.LotteryModeScheduled,
		Status: service.LotteryCampaignActive, StartsAt: startsAt, EndsAt: endsAt,
		DrawAt: &drawAt, PerUserLimit: 1,
		Prizes: []service.LotteryPrizeInput{{
			Name: "Subscription reward", PrizeType: service.LotteryPrizeSubscriptionCode,
			SubscriptionGroupID: &groupID, SubscriptionValidityDays: 30,
			ProbabilityBPS: 10000, Inventory: 1, IsEnabled: true,
		}},
	})
	require.NoError(t, err)

	var rewardCodeID int64
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_events WHERE campaign_id=$1`, campaign.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_entries WHERE campaign_id=$1`, campaign.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_draw_runs WHERE campaign_id=$1`, campaign.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_campaigns WHERE id=$1`, campaign.ID)
		if rewardCodeID > 0 {
			_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM redeem_codes WHERE id=$1`, rewardCodeID)
		}
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM groups WHERE id=$1`, groupID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})

	participation, err := repo.Participate(ctx, userID, campaign.ID, "scheduled-entry", time.Now().UTC(), fixedLotteryRandom{})
	require.NoError(t, err)
	require.False(t, participation.Replayed)
	require.Equal(t, service.LotteryEntryPending, participation.Entry.Status)

	type drawOutcome struct {
		result *service.LotteryDrawResult
		err    error
	}
	startDraw := make(chan struct{})
	drawOutcomes := make(chan drawOutcome, 2)
	for drawIndex := 0; drawIndex < 2; drawIndex++ {
		drawRepo := NewLotteryRepository(integrationDB)
		go func() {
			<-startDraw
			result, drawErr := drawRepo.DrawScheduled(ctx, campaign.ID, &userID, drawAt.Add(time.Second), fixedLotteryRandom{roll: 0})
			drawOutcomes <- drawOutcome{result: result, err: drawErr}
		}()
	}
	close(startDraw)
	freshDraws := 0
	replayedDraws := 0
	for drawIndex := 0; drawIndex < 2; drawIndex++ {
		outcome := <-drawOutcomes
		require.NoError(t, outcome.err)
		require.NotNil(t, outcome.result)
		require.Equal(t, 1, outcome.result.ParticipantCount)
		require.Equal(t, 1, outcome.result.WinnerCount)
		if outcome.result.AlreadyCompleted {
			replayedDraws++
		} else {
			freshDraws++
		}
	}
	require.Equal(t, 1, freshDraws)
	require.Equal(t, 1, replayedDraws)

	entries, total, err := repo.ListUserEntries(ctx, userID, service.LotteryListParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, entries, 1)
	require.Equal(t, service.LotteryEntryWon, entries[0].Status)
	require.Equal(t, "0123456789abcdef0123456789abcdef", entries[0].RewardCode)
	require.NotNil(t, entries[0].RewardRedeemCodeID)
	rewardCodeID = *entries[0].RewardRedeemCodeID

	var codeType, codeStatus string
	var codeGroupID int64
	var validityDays int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT type,status,group_id,validity_days
FROM redeem_codes WHERE id=$1`, rewardCodeID).Scan(&codeType, &codeStatus, &codeGroupID, &validityDays))
	require.Equal(t, service.RedeemTypeSubscription, codeType)
	require.Equal(t, service.StatusUnused, codeStatus)
	require.Equal(t, groupID, codeGroupID)
	require.Equal(t, 30, validityDays)

	var subscriptionCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_subscriptions
WHERE user_id=$1 AND group_id=$2`, userID, groupID).Scan(&subscriptionCount))
	require.Zero(t, subscriptionCount, "lottery must not bypass redeem-code subscription assignment")

	replayedDraw, err := repo.DrawScheduled(ctx, campaign.ID, &userID, drawAt.Add(time.Minute), fixedLotteryRandom{roll: 0})
	require.NoError(t, err)
	require.True(t, replayedDraw.AlreadyCompleted)
	var codeCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM redeem_codes WHERE id=$1`, rewardCodeID).Scan(&codeCount))
	require.Equal(t, 1, codeCount)
}

func TestLotteryRepositoryScheduledDrawSkipsInactiveEntriesWithoutConsumingRewards(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	userIDs := make([]int64, 4)
	for index := range userIDs {
		require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO users (email,password_hash,role,status,balance)
VALUES ($1,'test','user','active',0) RETURNING id`, fmt.Sprintf("lottery-eligibility-%d-%d@example.com", suffix, index)).Scan(&userIDs[index]))
	}
	var groupID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO groups (name,status,subscription_type)
VALUES ($1,'active','subscription') RETURNING id`, fmt.Sprintf("lottery-eligibility-subscription-%d", suffix)).Scan(&groupID))

	repo := NewLotteryRepository(integrationDB)
	now := time.Now().UTC()
	drawAt := now.Add(2 * time.Minute)
	campaign, err := repo.Create(ctx, userIDs[0], service.LotteryCampaignInput{
		Name: "Scheduled eligibility recheck", Mode: service.LotteryModeScheduled,
		Status: service.LotteryCampaignActive, StartsAt: now.Add(-time.Minute),
		EndsAt: now.Add(time.Minute), DrawAt: &drawAt, PerUserLimit: 1,
		Prizes: []service.LotteryPrizeInput{
			{
				Name: "Balance reward", PrizeType: service.LotteryPrizeBalance,
				BalanceAmount: 10, ProbabilityBPS: 5000, Inventory: 1, IsEnabled: true,
			},
			{
				Name: "Subscription reward", PrizeType: service.LotteryPrizeSubscriptionCode,
				SubscriptionGroupID: &groupID, SubscriptionValidityDays: 30,
				ProbabilityBPS: 5000, Inventory: 1, IsEnabled: true,
			},
		},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_events WHERE campaign_id=$1`, campaign.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_entries WHERE campaign_id=$1`, campaign.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_draw_runs WHERE campaign_id=$1`, campaign.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_campaigns WHERE id=$1`, campaign.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM redeem_codes WHERE notes=$1`, fmt.Sprintf("Lottery campaign %d reward", campaign.ID))
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM groups WHERE id=$1`, groupID)
		for _, userID := range userIDs {
			_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
		}
	})

	// The first two users become ineligible only after participation. They must
	// still be resolved, but cannot enter prize allocation or receive a reward.
	for index, userID := range userIDs {
		participation, participateErr := repo.Participate(ctx, userID, campaign.ID,
			fmt.Sprintf("eligibility-entry-%d", index), now, fixedLotteryRandom{})
		require.NoError(t, participateErr)
		require.Equal(t, service.LotteryEntryPending, participation.Entry.Status)
	}
	_, err = integrationDB.ExecContext(ctx, `UPDATE users SET status='disabled' WHERE id=$1`, userIDs[0])
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE users SET deleted_at=$2 WHERE id=$1`, userIDs[1], now)
	require.NoError(t, err)

	result, err := repo.DrawScheduled(ctx, campaign.ID, nil, drawAt.Add(time.Second), fixedLotteryRandom{roll: 0})
	require.NoError(t, err)
	require.False(t, result.AlreadyCompleted)
	require.Equal(t, len(userIDs), result.ParticipantCount)
	require.Equal(t, 2, result.WinnerCount)

	for _, userID := range userIDs[:2] {
		var status, prizeType string
		var rewardCodeID int64
		var balanceAmount, userBalance float64
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT status,COALESCE(prize_type,''),
COALESCE(reward_redeem_code_id,0),balance_amount FROM lottery_entries
WHERE campaign_id=$1 AND user_id=$2`, campaign.ID, userID).Scan(&status, &prizeType, &rewardCodeID, &balanceAmount))
		require.Equal(t, service.LotteryEntryNotWon, status)
		require.Empty(t, prizeType)
		require.Zero(t, rewardCodeID)
		require.Zero(t, balanceAmount)
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT balance FROM users WHERE id=$1`, userID).Scan(&userBalance))
		require.Zero(t, userBalance)
	}

	var awardedCount, wonCount, codeCount int
	var activeBalance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COALESCE(SUM(awarded_count),0)
FROM lottery_prizes WHERE campaign_id=$1`, campaign.ID).Scan(&awardedCount))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM lottery_entries
WHERE campaign_id=$1 AND status='won'`, campaign.ID).Scan(&wonCount))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM lottery_entries
WHERE campaign_id=$1 AND reward_redeem_code_id IS NOT NULL`, campaign.ID).Scan(&codeCount))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COALESCE(SUM(balance),0) FROM users
WHERE id IN ($1,$2)`, userIDs[2], userIDs[3]).Scan(&activeBalance))
	require.Equal(t, 2, awardedCount)
	require.Equal(t, 2, wonCount)
	require.Equal(t, 1, codeCount)
	require.Equal(t, 10.0, activeBalance)

	replayed, err := repo.DrawScheduled(ctx, campaign.ID, nil, drawAt.Add(time.Minute), fixedLotteryRandom{roll: 0})
	require.NoError(t, err)
	require.True(t, replayed.AlreadyCompleted)
	require.Equal(t, 2, replayed.WinnerCount)
}

func TestLotteryRepositoryFullDrawCountsUniqueUsersAndClosesAtCapacity(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	userIDs := make([]int64, 4)
	for index := range userIDs {
		require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO users (email,password_hash,role,status,balance)
VALUES ($1,'test','user','active',0) RETURNING id`, fmt.Sprintf("lottery-full-%d-%d@example.com", suffix, index)).Scan(&userIDs[index]))
	}

	repo := NewLotteryRepository(integrationDB)
	now := time.Now().UTC()
	endsAt := now.Add(time.Hour)
	drawAt := endsAt.Add(time.Hour)
	participantLimit := 3
	campaign, err := repo.Create(ctx, userIDs[0], service.LotteryCampaignInput{
		Name: "Full draw capacity test", Mode: service.LotteryModeScheduled,
		Status: service.LotteryCampaignActive, StartsAt: now.Add(-time.Minute), EndsAt: endsAt,
		DrawAt: &drawAt, FullDrawParticipantLimit: &participantLimit, PerUserLimit: 2,
		Prizes: []service.LotteryPrizeInput{{
			Name: "Capacity reward", PrizeType: service.LotteryPrizeBalance,
			BalanceAmount: 1, ProbabilityBPS: 10000, Inventory: 3, IsEnabled: true,
		}},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_events WHERE campaign_id=$1`, campaign.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_entries WHERE campaign_id=$1`, campaign.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_draw_runs WHERE campaign_id=$1`, campaign.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_campaigns WHERE id=$1`, campaign.ID)
		for _, userID := range userIDs {
			_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
		}
	})

	first, err := repo.Participate(ctx, userIDs[0], campaign.ID, "user-0-entry-1", now, fixedLotteryRandom{})
	require.NoError(t, err)
	require.False(t, first.FullDrawTriggered)
	second, err := repo.Participate(ctx, userIDs[0], campaign.ID, "user-0-entry-2", now, fixedLotteryRandom{})
	require.NoError(t, err)
	require.False(t, second.FullDrawTriggered, "a repeat participant must not consume another participant slot")

	type participationOutcome struct {
		userID int64
		key    string
		result *service.LotteryParticipationResult
		err    error
	}
	outcomes := make(chan participationOutcome, 3)
	var waitGroup sync.WaitGroup
	for index := 1; index < len(userIDs); index++ {
		waitGroup.Add(1)
		go func(userID int64, key string) {
			defer waitGroup.Done()
			result, participateErr := repo.Participate(ctx, userID, campaign.ID, key, time.Now().UTC(), fixedLotteryRandom{})
			outcomes <- participationOutcome{userID: userID, key: key, result: result, err: participateErr}
		}(userIDs[index], fmt.Sprintf("user-%d-entry", index))
	}
	waitGroup.Wait()
	close(outcomes)

	successes := make([]participationOutcome, 0, 2)
	capacityErrors := 0
	triggered := 0
	for outcome := range outcomes {
		if outcome.err != nil {
			require.ErrorIs(t, outcome.err, service.ErrLotteryCapacityReached)
			capacityErrors++
			continue
		}
		successes = append(successes, outcome)
		if outcome.result.FullDrawTriggered {
			triggered++
		}
	}
	require.Len(t, successes, 2)
	require.Equal(t, 1, capacityErrors)
	require.Equal(t, 1, triggered)

	fullCampaign, err := repo.GetForAdmin(ctx, campaign.ID, time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, 3, fullCampaign.ParticipantCount)
	require.Equal(t, 4, fullCampaign.EntryCount)
	require.NotNil(t, fullCampaign.FullDrawReachedAt)
	require.Equal(t, "awaiting_draw", fullCampaign.State)

	replayed, err := repo.Participate(ctx, successes[0].userID, campaign.ID, successes[0].key, time.Now().UTC(), fixedLotteryRandom{})
	require.NoError(t, err)
	require.True(t, replayed.Replayed, "idempotent retries must still work after capacity is reached")

	dueIDs, err := repo.ListDueScheduledCampaignIDs(ctx, time.Now().UTC(), 20)
	require.NoError(t, err)
	require.Contains(t, dueIDs, campaign.ID, "a full campaign must be due before its scheduled draw time")

	drawResult, err := repo.DrawScheduled(ctx, campaign.ID, nil, time.Now().UTC(), fixedLotteryRandom{roll: 0})
	require.NoError(t, err)
	require.Equal(t, 4, drawResult.ParticipantCount)
	require.Equal(t, 3, drawResult.WinnerCount)

	var trigger string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT metadata->>'trigger' FROM lottery_events
WHERE campaign_id=$1 AND event_type='scheduled_draw_completed'`, campaign.ID).Scan(&trigger))
	require.Equal(t, "capacity", trigger)
}

func TestLotteryRepositoryRejectsInvalidEligibilitySubscriptionGroup(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	var userID, groupID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO users (email,password_hash,role,status,balance)
VALUES ($1,'test','admin','active',0) RETURNING id`, fmt.Sprintf("lottery-eligibility-%d@example.com", suffix)).Scan(&userID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO groups (name,status,subscription_type)
VALUES ($1,'active','standard') RETURNING id`, fmt.Sprintf("lottery-standard-%d", suffix)).Scan(&groupID))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM groups WHERE id=$1`, groupID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})

	now := time.Now().UTC()
	_, err := NewLotteryRepository(integrationDB).Create(ctx, userID, service.LotteryCampaignInput{
		Name: "Invalid eligibility group", Mode: service.LotteryModeInstant,
		Status: service.LotteryCampaignDraft, StartsAt: now, EndsAt: now.Add(time.Hour),
		PerUserLimit: 1, RequiredSubscriptionGroupIDs: []int64{groupID},
		Prizes: []service.LotteryPrizeInput{{
			Name: "Balance reward", PrizeType: service.LotteryPrizeBalance,
			BalanceAmount: 1, ProbabilityBPS: 1000, Inventory: 1, IsEnabled: true,
		}},
	})
	require.ErrorIs(t, err, service.ErrLotteryInvalid)
}

func TestLotteryRepositoryStatusTransitionsRemainTerminalAndRejectExpiredActivation(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	var userID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO users (email,password_hash,role,status,balance)
VALUES ($1,'test','admin','active',0) RETURNING id`, fmt.Sprintf("lottery-status-%d@example.com", suffix)).Scan(&userID))

	repo := NewLotteryRepository(integrationDB)
	now := time.Now().UTC()
	campaign, err := repo.Create(ctx, userID, service.LotteryCampaignInput{
		Name: "Status transition test", Mode: service.LotteryModeInstant,
		Status: service.LotteryCampaignDraft, StartsAt: now.Add(-2 * time.Hour),
		EndsAt: now.Add(-time.Hour), PerUserLimit: 1,
		Prizes: []service.LotteryPrizeInput{{
			Name: "Balance reward", PrizeType: service.LotteryPrizeBalance,
			BalanceAmount: 1, ProbabilityBPS: 1000, Inventory: 1, IsEnabled: true,
		}},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_campaigns WHERE id=$1`, campaign.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})

	_, err = repo.SetStatus(ctx, campaign.ID, userID, service.LotteryCampaignActive, now)
	require.ErrorIs(t, err, service.ErrLotteryEnded)

	completed, err := repo.SetStatus(ctx, campaign.ID, userID, service.LotteryCampaignCompleted, now)
	require.NoError(t, err)
	require.Equal(t, service.LotteryCampaignCompleted, completed.Status)

	_, err = repo.SetStatus(ctx, campaign.ID, userID, service.LotteryCampaignActive, now)
	require.ErrorIs(t, err, service.ErrLotteryInvalidTransition)
}

func TestLotteryRepositoryScheduledCampaignMustCompleteThroughDraw(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	var userID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO users (email,password_hash,role,status,balance)
VALUES ($1,'test','admin','active',0) RETURNING id`, fmt.Sprintf("lottery-scheduled-status-%d@example.com", suffix)).Scan(&userID))

	repo := NewLotteryRepository(integrationDB)
	now := time.Now().UTC()
	drawAt := now.Add(-30 * time.Minute)
	campaign, err := repo.Create(ctx, userID, service.LotteryCampaignInput{
		Name: "Scheduled status transition", Mode: service.LotteryModeScheduled,
		Status: service.LotteryCampaignPaused, StartsAt: now.Add(-2 * time.Hour),
		EndsAt: now.Add(-time.Hour), DrawAt: &drawAt, PerUserLimit: 1,
		Prizes: []service.LotteryPrizeInput{{
			Name: "Balance reward", PrizeType: service.LotteryPrizeBalance,
			BalanceAmount: 1, ProbabilityBPS: 1000, Inventory: 1, IsEnabled: true,
		}},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM lottery_campaigns WHERE id=$1`, campaign.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})

	active, err := repo.SetStatus(ctx, campaign.ID, userID, service.LotteryCampaignActive, now)
	require.NoError(t, err, "scheduled campaigns must remain activatable for their draw after entries close")
	require.Equal(t, service.LotteryCampaignActive, active.Status)

	_, err = repo.SetStatus(ctx, campaign.ID, userID, service.LotteryCampaignCompleted, now)
	require.ErrorIs(t, err, service.ErrLotteryInvalidTransition)
}
