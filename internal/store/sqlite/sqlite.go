// Package sqlite implements store.Store using modernc.org/sqlite (pure Go, no CGO).
//
// 文档模式：复杂对象序列化为 JSON 存入 data 列，索引列仅用于查询/排序。
// 迁移 SQL 内嵌，启动时按 schema_version 追踪执行。
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"boxpanel/internal/models"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store is the SQLite implementation of store.Store.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and runs migrations.
func Open(path string) (*Store, error) {
	// busy_timeout 防止并发写时 SQLITE_BUSY；foreign_keys 开启外键。
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// 单写连接：SQLite 写串行化，避免锁冲突。
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	// 读取内嵌迁移文件并按顺序执行（幂等 IF NOT EXISTS + 版本表）。
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, e := range entries {
		sqlBytes, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(string(sqlBytes)); err != nil {
			return fmt.Errorf("apply %s: %w", e.Name(), err)
		}
	}
	// 记录版本
	for i := range entries {
		_, _ = s.db.Exec(
			"INSERT OR IGNORE INTO schema_version(version, applied) VALUES(?, ?)",
			i+1, time.Now().Format(time.RFC3339),
		)
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

// ============================================================
//  Servers
// ============================================================

func (s *Store) ListServers(ctx context.Context) ([]models.Server, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT data FROM servers ORDER BY added_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Server
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var srv models.Server
		if err := json.Unmarshal([]byte(data), &srv); err != nil {
			continue
		}
		out = append(out, srv)
	}
	return out, rows.Err()
}

func (s *Store) GetServer(ctx context.Context, id string) (*models.Server, error) {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT data FROM servers WHERE id = ?`, id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var srv models.Server
	if err := json.Unmarshal([]byte(data), &srv); err != nil {
		return nil, err
	}
	return &srv, nil
}

func (s *Store) SaveServer(ctx context.Context, srv models.Server) error {
	if srv.AddedAt.IsZero() {
		srv.AddedAt = time.Now()
	}
	data, err := json.Marshal(srv)
	if err != nil {
		return err
	}
	latency := (*int)(nil)
	if srv.LastLatency != nil {
		v := *srv.LastLatency
		latency = &v
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO servers (id, protocol, name, server, server_port, last_latency_ms, added_at, data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			protocol=excluded.protocol, name=excluded.name, server=excluded.server,
			server_port=excluded.server_port, last_latency_ms=excluded.last_latency_ms,
			added_at=excluded.added_at, data=excluded.data`,
		srv.ID, srv.Protocol, srv.Name, srv.Server, srv.ServerPort, latency,
		srv.AddedAt.Format(time.RFC3339), string(data),
	)
	return err
}

func (s *Store) BatchSaveServers(ctx context.Context, servers []models.Server) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO servers (id, protocol, name, server, server_port, last_latency_ms, added_at, data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			protocol=excluded.protocol, name=excluded.name, server=excluded.server,
			server_port=excluded.server_port, last_latency_ms=excluded.last_latency_ms,
			added_at=excluded.added_at, data=excluded.data`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, srv := range servers {
		if srv.AddedAt.IsZero() {
			srv.AddedAt = time.Now()
		}
		data, err := json.Marshal(srv)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, srv.ID, srv.Protocol, srv.Name, srv.Server,
			srv.ServerPort, srv.LastLatency, srv.AddedAt.Format(time.RFC3339), string(data)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteServer(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM servers WHERE id = ?`, id)
	return err
}

// ============================================================
//  Groups
// ============================================================

func (s *Store) ListGroups(ctx context.Context) ([]models.Group, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM groups ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Group
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var g models.Group
		if err := json.Unmarshal([]byte(data), &g); err != nil {
			continue
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) GetGroup(ctx context.Context, id string) (*models.Group, error) {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT data FROM groups WHERE id = ?`, id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var g models.Group
	if err := json.Unmarshal([]byte(data), &g); err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Store) SaveGroup(ctx context.Context, g models.Group) error {
	if g.AddedAt.IsZero() {
		g.AddedAt = time.Now()
	}
	data, err := json.Marshal(g)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO groups (id, name, type, data) VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, type=excluded.type, data=excluded.data`,
		g.ID, g.Name, g.Type, string(data))
	return err
}

func (s *Store) DeleteGroup(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM groups WHERE id = ?`, id)
	return err
}

// ============================================================
//  Subscriptions
// ============================================================

func (s *Store) ListSubscriptions(ctx context.Context) ([]models.Subscription, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM subscriptions ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Subscription
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var sub models.Subscription
		if err := json.Unmarshal([]byte(data), &sub); err != nil {
			continue
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *Store) GetSubscription(ctx context.Context, id string) (*models.Subscription, error) {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT data FROM subscriptions WHERE id = ?`, id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var sub models.Subscription
	if err := json.Unmarshal([]byte(data), &sub); err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Store) SaveSubscription(ctx context.Context, sub models.Subscription) error {
	if sub.AddedAt.IsZero() {
		sub.AddedAt = time.Now()
	}
	if sub.IntervalHours == 0 {
		sub.IntervalHours = 24
	}
	lastRefresh := ""
	if !sub.LastRefresh.IsZero() {
		lastRefresh = sub.LastRefresh.Format(time.RFC3339)
	}
	data, err := json.Marshal(sub)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, name, url, last_refresh, last_status, data) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, url=excluded.url,
			last_refresh=excluded.last_refresh, last_status=excluded.last_status, data=excluded.data`,
		sub.ID, sub.Name, sub.URL, lastRefresh, sub.LastStatus, string(data))
	return err
}

func (s *Store) DeleteSubscription(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM subscriptions WHERE id = ?`, id)
	return err
}

// ============================================================
//  Routing rules
// ============================================================

func (s *Store) ListRoutingRules(ctx context.Context, profileID string) ([]models.RoutingRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT data FROM routing_rules WHERE profile_id = ? ORDER BY ord ASC`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.RoutingRule
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var r models.RoutingRule
		if err := json.Unmarshal([]byte(data), &r); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) SaveRoutingRule(ctx context.Context, r models.RoutingRule) error {
	if r.AddedAt.IsZero() {
		r.AddedAt = time.Now()
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO routing_rules (id, profile_id, ord, type, outbound, data) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET profile_id=excluded.profile_id, ord=excluded.ord,
			type=excluded.type, outbound=excluded.outbound, data=excluded.data`,
		r.ID, r.ProfileID, r.Order, r.Type, r.Outbound, string(data))
	return err
}

func (s *Store) DeleteRoutingRule(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM routing_rules WHERE id = ?`, id)
	return err
}

func (s *Store) ReorderRoutingRules(ctx context.Context, profileID string, ids []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `UPDATE routing_rules SET ord = ? WHERE id = ? AND profile_id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i, id := range ids {
		if _, err := stmt.ExecContext(ctx, i, id, profileID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ============================================================
//  Rule sets
// ============================================================

func (s *Store) ListRuleSets(ctx context.Context) ([]models.RuleSet, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM rule_sets ORDER BY tag ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.RuleSet
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var rs models.RuleSet
		if err := json.Unmarshal([]byte(data), &rs); err != nil {
			continue
		}
		out = append(out, rs)
	}
	return out, rows.Err()
}

func (s *Store) GetRuleSet(ctx context.Context, id string) (*models.RuleSet, error) {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT data FROM rule_sets WHERE id = ?`, id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rs models.RuleSet
	if err := json.Unmarshal([]byte(data), &rs); err != nil {
		return nil, err
	}
	return &rs, nil
}

func (s *Store) SaveRuleSet(ctx context.Context, r models.RuleSet) error {
	if r.AddedAt.IsZero() {
		r.AddedAt = time.Now()
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO rule_sets (id, tag, type, data) VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET tag=excluded.tag, type=excluded.type, data=excluded.data`,
		r.ID, r.Tag, r.Type, string(data))
	return err
}

func (s *Store) DeleteRuleSet(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM rule_sets WHERE id = ?`, id)
	return err
}

// ============================================================
//  Profiles
// ============================================================

func (s *Store) ListProfiles(ctx context.Context) ([]models.Profile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM profiles ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Profile
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var p models.Profile
		if err := json.Unmarshal([]byte(data), &p); err != nil {
			continue
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetProfile(ctx context.Context, id string) (*models.Profile, error) {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT data FROM profiles WHERE id = ?`, id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var p models.Profile
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) SaveProfile(ctx context.Context, p models.Profile) error {
	if p.AddedAt.IsZero() {
		p.AddedAt = time.Now()
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO profiles (id, name, mode, data) VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, mode=excluded.mode, data=excluded.data`,
		p.ID, p.Name, p.Mode, string(data))
	return err
}

func (s *Store) DeleteProfile(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM profiles WHERE id = ?`, id)
	return err
}

// ============================================================
//  Settings (KV)
// ============================================================

func (s *Store) GetSettings(ctx context.Context) (models.Settings, error) {
	st := models.DefaultSettings()
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	kv := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return st, err
		}
		kv[k] = v
	}
	// 反序列化整个 settings 为 JSON 再覆盖默认值
	if blob, ok := kv["__settings__"]; ok {
		_ = json.Unmarshal([]byte(blob), &st)
	}
	return st, nil
}

func (s *Store) SaveSettings(ctx context.Context, st models.Settings) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value) VALUES ('__settings__', ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(data))
	return err
}
