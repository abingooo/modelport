package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type modelCatalogRepository struct {
	db *sql.DB
}

func NewModelCatalogRepository(db *sql.DB) service.ModelCatalogRepository {
	return &modelCatalogRepository{db: db}
}

func (r *modelCatalogRepository) List(ctx context.Context) ([]service.ModelCatalogMetadata, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, platform, model_name, display_name, description, capabilities,
		       context_window, interface_formats, scenarios, example_overrides,
		       is_recommended, is_visible, sort_order, created_at, updated_at
		FROM model_catalog_metadata
		ORDER BY is_recommended DESC, sort_order, platform, model_name`)
	if err != nil {
		return nil, fmt.Errorf("list model catalog metadata: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.ModelCatalogMetadata, 0)
	for rows.Next() {
		var metadata service.ModelCatalogMetadata
		var capabilities, formats, scenarios, examples []byte
		if err := rows.Scan(
			&metadata.ID, &metadata.Platform, &metadata.ModelName, &metadata.DisplayName,
			&metadata.Description, &capabilities, &metadata.ContextWindow, &formats,
			&scenarios, &examples, &metadata.IsRecommended, &metadata.IsVisible,
			&metadata.SortOrder, &metadata.CreatedAt, &metadata.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan model catalog metadata: %w", err)
		}
		if err := json.Unmarshal(capabilities, &metadata.Capabilities); err != nil {
			return nil, fmt.Errorf("decode model catalog capabilities: %w", err)
		}
		if err := json.Unmarshal(formats, &metadata.InterfaceFormats); err != nil {
			return nil, fmt.Errorf("decode model catalog interface formats: %w", err)
		}
		if err := json.Unmarshal(scenarios, &metadata.Scenarios); err != nil {
			return nil, fmt.Errorf("decode model catalog scenarios: %w", err)
		}
		if err := json.Unmarshal(examples, &metadata.ExampleOverrides); err != nil {
			return nil, fmt.Errorf("decode model catalog examples: %w", err)
		}
		out = append(out, metadata)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate model catalog metadata: %w", err)
	}
	return out, nil
}

func (r *modelCatalogRepository) Upsert(ctx context.Context, metadata *service.ModelCatalogMetadata) error {
	capabilities, err := json.Marshal(metadata.Capabilities)
	if err != nil {
		return fmt.Errorf("encode model catalog capabilities: %w", err)
	}
	formats, err := json.Marshal(metadata.InterfaceFormats)
	if err != nil {
		return fmt.Errorf("encode model catalog interface formats: %w", err)
	}
	scenarios, err := json.Marshal(metadata.Scenarios)
	if err != nil {
		return fmt.Errorf("encode model catalog scenarios: %w", err)
	}
	examples, err := json.Marshal(metadata.ExampleOverrides)
	if err != nil {
		return fmt.Errorf("encode model catalog examples: %w", err)
	}

	err = r.db.QueryRowContext(ctx, `
		INSERT INTO model_catalog_metadata (
			platform, model_name, display_name, description, capabilities, context_window,
			interface_formats, scenarios, example_overrides, is_recommended, is_visible, sort_order
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (platform, LOWER(model_name)) DO UPDATE SET
			display_name = EXCLUDED.display_name,
			description = EXCLUDED.description,
			capabilities = EXCLUDED.capabilities,
			context_window = EXCLUDED.context_window,
			interface_formats = EXCLUDED.interface_formats,
			scenarios = EXCLUDED.scenarios,
			example_overrides = EXCLUDED.example_overrides,
			is_recommended = EXCLUDED.is_recommended,
			is_visible = EXCLUDED.is_visible,
			sort_order = EXCLUDED.sort_order,
			updated_at = NOW()
		RETURNING id, created_at, updated_at`,
		metadata.Platform, metadata.ModelName, metadata.DisplayName, metadata.Description,
		capabilities, metadata.ContextWindow, formats, scenarios, examples,
		metadata.IsRecommended, metadata.IsVisible, metadata.SortOrder,
	).Scan(&metadata.ID, &metadata.CreatedAt, &metadata.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert model catalog metadata: %w", err)
	}
	return nil
}

func (r *modelCatalogRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM model_catalog_metadata WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete model catalog metadata: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted model catalog rows: %w", err)
	}
	if rows == 0 {
		return service.ErrModelCatalogMetadataNotFound
	}
	return nil
}
