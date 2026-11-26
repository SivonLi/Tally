package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// SavedView 保存的界面视图（筛选、列配置、排序）。
type SavedView struct {
	ID          int64     `json:"id"`
	Page        string    `json:"page"`
	Name        string    `json:"name"`
	FiltersJSON string    `json:"filtersJson"`
	ColumnsJSON string    `json:"columnsJson"`
	SortJSON    string    `json:"sortJson"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type savedViewPayload struct {
	FiltersJSON string `json:"filters_json"`
	ColumnsJSON string `json:"columns_json"`
	SortJSON    string `json:"sort_json"`
	UpdatedAt   string `json:"updated_at"`
}

// SaveView 保存或更新一个命名视图。
func (s *Store) SaveView(ctx context.Context, view SavedView) (int64, error) {
	if view.Page == "" || view.Name == "" {
		return 0, fmt.Errorf("页面和视图名称不能为空")
	}
	if view.FiltersJSON == "" {
		view.FiltersJSON = "{}"
	}
	if view.ColumnsJSON == "" {
		view.ColumnsJSON = "[]"
	}
	if view.SortJSON == "" {
		view.SortJSON = "{}"
	}
	now := s.now().Format(time.RFC3339Nano)
	encoded, err := json.Marshal(savedViewPayload{
		FiltersJSON: view.FiltersJSON,
		ColumnsJSON: view.ColumnsJSON,
		SortJSON:    view.SortJSON,
		UpdatedAt:   now,
	})
	if err != nil {
		return 0, err
	}
	if view.ID > 0 {
		_, err := s.db.Execute(ctx, `UPDATE saved_views SET name = ?, config_json = ? WHERE id = ?`,
			view.Name, string(encoded), view.ID)
		if err != nil {
			return 0, err
		}
		return view.ID, nil
	}
	res, err := s.db.Execute(ctx, `INSERT INTO saved_views(page, name, config_json, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(page, name) DO UPDATE SET config_json = excluded.config_json`,
		view.Page, view.Name, string(encoded), now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertID, nil
}

// ListViews 列出指定页面的所有已保存视图。
func (s *Store) ListViews(ctx context.Context, page string) ([]SavedView, error) {
	rows, err := s.db.Query(ctx, `SELECT id, page, name, config_json, created_at
		FROM saved_views WHERE page = ? ORDER BY name ASC`, page)
	if err != nil {
		return nil, err
	}
	views := make([]SavedView, 0, len(rows.Values))
	for _, row := range rows.Values {
		created, err := parseTime(row[4])
		if err != nil {
			return nil, err
		}
		var payload savedViewPayload
		if raw := stringValue(row[3]); raw != "" {
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				return nil, err
			}
		}
		if payload.FiltersJSON == "" {
			payload.FiltersJSON = "{}"
		}
		if payload.ColumnsJSON == "" {
			payload.ColumnsJSON = "[]"
		}
		if payload.SortJSON == "" {
			payload.SortJSON = "{}"
		}
		updated := created
		if payload.UpdatedAt != "" {
			if parsed, err := parseTime(payload.UpdatedAt); err == nil {
				updated = parsed
			}
		}
		views = append(views, SavedView{
			ID:          intValue(row[0]),
			Page:        stringValue(row[1]),
			Name:        stringValue(row[2]),
			FiltersJSON: payload.FiltersJSON,
			ColumnsJSON: payload.ColumnsJSON,
			SortJSON:    payload.SortJSON,
			CreatedAt:   created,
			UpdatedAt:   updated,
		})
	}
	return views, nil
}

// DeleteView 删除一个视图。
func (s *Store) DeleteView(ctx context.Context, id int64) error {
	_, err := s.db.Execute(ctx, `DELETE FROM saved_views WHERE id = ?`, id)
	return err
}
