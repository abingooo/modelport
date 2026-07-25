//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type bulkGroupAPIKeyRepoStub struct {
	APIKeyRepository
	calledUserID  int64
	calledKeyIDs  []int64
	calledGroupID int64
	keys          []string
	updated       int64
	err           error
}

func (s *bulkGroupAPIKeyRepoStub) UpdateGroupIDByUserAndIDs(_ context.Context, userID int64, keyIDs []int64, groupID int64) ([]string, int64, error) {
	s.calledUserID = userID
	s.calledKeyIDs = append([]int64(nil), keyIDs...)
	s.calledGroupID = groupID
	return append([]string(nil), s.keys...), s.updated, s.err
}

type bulkGroupUserRepoStub struct {
	UserRepository
	user *User
	err  error
}

func (s *bulkGroupUserRepoStub) GetByID(context.Context, int64) (*User, error) {
	return s.user, s.err
}

type bulkGroupGroupRepoStub struct {
	GroupRepository
	group *Group
	err   error
}

func (s *bulkGroupGroupRepoStub) GetByID(context.Context, int64) (*Group, error) {
	return s.group, s.err
}

func TestAPIKeyServiceBulkUpdateGroupDeduplicatesAndInvalidates(t *testing.T) {
	repo := &bulkGroupAPIKeyRepoStub{
		keys:    []string{"sk-bulk-one", "sk-bulk-two"},
		updated: 2,
	}
	cache := &apiKeyCacheStub{}
	svc := &APIKeyService{
		apiKeyRepo: repo,
		userRepo:   &bulkGroupUserRepoStub{user: &User{ID: 7, Status: StatusActive}},
		groupRepo:  &bulkGroupGroupRepoStub{group: &Group{ID: 42, Status: StatusActive}},
		cache:      cache,
	}

	updated, err := svc.BulkUpdateGroup(context.Background(), 7, []int64{11, 11, 12}, 42)
	require.NoError(t, err)
	require.Equal(t, int64(2), updated)
	require.Equal(t, int64(7), repo.calledUserID)
	require.Equal(t, []int64{11, 12}, repo.calledKeyIDs)
	require.Equal(t, int64(42), repo.calledGroupID)
	require.ElementsMatch(t, []string{
		svc.authCacheKey("sk-bulk-one"),
		svc.authCacheKey("sk-bulk-two"),
	}, cache.deleteAuthKeys)
}

func TestAPIKeyServiceBulkUpdateGroupRejectsForbiddenExclusiveGroup(t *testing.T) {
	repo := &bulkGroupAPIKeyRepoStub{}
	svc := &APIKeyService{
		apiKeyRepo: repo,
		userRepo:   &bulkGroupUserRepoStub{user: &User{ID: 7, Status: StatusActive}},
		groupRepo: &bulkGroupGroupRepoStub{group: &Group{
			ID:          42,
			Status:      StatusActive,
			IsExclusive: true,
		}},
	}

	_, err := svc.BulkUpdateGroup(context.Background(), 7, []int64{11}, 42)
	require.ErrorIs(t, err, ErrGroupNotAllowed)
	require.Empty(t, repo.calledKeyIDs)
}

func TestAPIKeyServiceBulkUpdateGroupValidatesSelectionSize(t *testing.T) {
	svc := &APIKeyService{}

	_, err := svc.BulkUpdateGroup(context.Background(), 7, nil, 42)
	require.ErrorIs(t, err, ErrAPIKeyBulkEmpty)

	tooMany := make([]int64, MaxBulkAPIKeyGroupUpdates+1)
	for index := range tooMany {
		tooMany[index] = int64(index + 1)
	}
	_, err = svc.BulkUpdateGroup(context.Background(), 7, tooMany, 42)
	require.ErrorIs(t, err, ErrAPIKeyBulkTooLarge)
}
