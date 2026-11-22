package store

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
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

func savedViewsSchema() []sqlite.Statement {
	return []sqlite.Statement{
		{SQL: `CREATE TABLE IF NOT EXISTS saved_views (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			page TEXT NOT NULL,
			name TEXT NOT NULL,
			filters_json TEXT NOT NULL DEFAULT '{}',
			columns_json TEXT NOT NULL DEFAULT '[]',
			sort_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`},
		{SQL: `CREATE UNIQUE INDEX IF NOT EXISTS saved_views_page_name
			ON saved_views(page, name)`},
	}
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
	if view.ID > 0 {
		_, err := s.db.Execute(ctx, `UPDATE saved_views
			SET name = ?, filters_json = ?, columns_json = ?, sort_json = ?, updated_at = ?
			WHERE id = ?`,
			view.Name, view.FiltersJSON, view.ColumnsJSON, view.SortJSON, now, view.ID)
		if err != nil {
			return 0, err
		}
		return view.ID, nil
	}
	res, err := s.db.Execute(ctx, `INSERT INTO saved_views (page, name, filters_json, columns_json, sort_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(page, name) DO UPDATE SET
			filters_json = excluded.filters_json,
			columns_json = excluded.columns_json,
			sort_json = excluded.sort_json,
			updated_at = excluded.updated_at`,
		view.Page, view.Name, view.FiltersJSON, view.ColumnsJSON, view.SortJSON, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertID, nil
}

// ListViews 列出指定页面的所有已保存视图。
func (s *Store) ListViews(ctx context.Context, page string) ([]SavedView, error) {
	rows, err := s.db.Query(ctx, `SELECT id, page, name, filters_json, columns_json, sort_json, created_at, updated_at
		FROM saved_views WHERE page = ? ORDER BY name ASC`, page)
	if err != nil {
		return nil, err
	}
	views := make([]SavedView, 0, len(rows.Values))
	for _, row := range rows.Values {
		id, _ := row[0].(int64)
		pg, _ := row[1].(string)
		name, _ := row[2].(string)
		filters, _ := row[3].(string)
		columns, _ := row[4].(string)
		sort, _ := row[5].(string)
		createdAtStr, _ := row[6].(string)
		updatedAtStr, _ := row[7].(string)
		created, _ := time.Parse(time.RFC3339Nano, createdAtStr)
		updated, _ := time.Parse(time.RFC3339Nano, updatedAtStr)
		views = append(views, SavedView{
			ID:          id,
			Page:        pg,
			Name:        name,
			FiltersJSON: filters,
			ColumnsJSON: columns,
			SortJSON:    sort,
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
