package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestModelCatalogRepositoryListDecodesMetadata(t *testing.T) {
	database, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	repository := NewModelCatalogRepository(database)
	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "platform", "model_name", "display_name", "description", "capabilities",
		"context_window", "interface_formats", "scenarios", "example_overrides",
		"is_recommended", "is_visible", "sort_order", "created_at", "updated_at",
	}).AddRow(
		int64(1), service.PlatformOpenAI, "gpt-5", "GPT 5", "description", []byte(`["text","reasoning"]`),
		int64(400000), []byte(`["openai"]`), []byte(`["chat"]`), []byte(`{"openai":"curl example"}`),
		true, true, 5, now, now,
	)
	mock.ExpectQuery("SELECT id, platform, model_name").WillReturnRows(rows)

	items, err := repository.List(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, []string{"text", "reasoning"}, items[0].Capabilities)
	require.Equal(t, "curl example", items[0].ExampleOverrides["openai"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelCatalogRepositoryDeleteReportsMissingMetadata(t *testing.T) {
	database, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	repository := NewModelCatalogRepository(database)
	mock.ExpectExec("DELETE FROM model_catalog_metadata").WithArgs(int64(77)).WillReturnResult(sqlmock.NewResult(0, 0))

	err = repository.Delete(context.Background(), 77)
	require.ErrorIs(t, err, service.ErrModelCatalogMetadataNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModelCatalogRepositoryUpsertUsesCaseInsensitiveIdentity(t *testing.T) {
	database, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	repository := NewModelCatalogRepository(database)
	now := time.Now()
	metadata := &service.ModelCatalogMetadata{
		Platform: service.PlatformMiniMax, ModelName: "MiniMax-M2.7", DisplayName: "MiniMax M2.7",
		Capabilities: []string{"text"}, InterfaceFormats: []string{"openai"},
		Scenarios: []string{"chat"}, ExampleOverrides: map[string]string{}, IsVisible: true,
	}
	mock.ExpectQuery(`ON CONFLICT \(platform, LOWER\(model_name\)\)`).
		WithArgs(
			metadata.Platform, metadata.ModelName, metadata.DisplayName, metadata.Description,
			sqlmock.AnyArg(), metadata.ContextWindow, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			metadata.IsRecommended, metadata.IsVisible, metadata.SortOrder,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(int64(7), now, now))

	require.NoError(t, repository.Upsert(context.Background(), metadata))
	require.Equal(t, int64(7), metadata.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}
