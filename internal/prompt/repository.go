package prompt

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/ti/router/tibrain/internal/ports"
)

var (
	ErrCapsuleNotFound = fmt.Errorf("capsule not found")
	ErrVersionNotFound = fmt.Errorf("version not found")
)

type sqliteRepository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) Repository {
	return &sqliteRepository{db: db}
}

func (r *sqliteRepository) DB() *sql.DB {
	return r.db
}

func (r *sqliteRepository) CreateCapsule(ctx context.Context, c PromptCapsule) error {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	now := timeNow()
	if c.CreatedAt == 0 {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO prompt_capsules (id, name, description, intent, domain, risk, status, source_url, source_type, retrieved_at, license_status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID, c.Name, c.Description, c.Intent, c.Domain, c.Risk, c.Status,
		c.SourceURL, c.SourceType, c.RetrievedAt, c.LicenseStatus, c.CreatedAt, c.UpdatedAt)
	return err
}

func (r *sqliteRepository) GetCapsule(ctx context.Context, id string) (*PromptCapsule, error) {
	var c PromptCapsule
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, description, intent, domain, risk, status, source_url, source_type, retrieved_at, license_status, created_at, updated_at
		 FROM prompt_capsules WHERE id = ?`, id).Scan(
		&c.ID, &c.Name, &c.Description, &c.Intent, &c.Domain, &c.Risk, &c.Status,
		&c.SourceURL, &c.SourceType, &c.RetrievedAt, &c.LicenseStatus, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrCapsuleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get capsule: %w", err)
	}
	return &c, nil
}

func (r *sqliteRepository) ListCapsules(ctx context.Context, intent, domain string, status CapsuleStatus, limit int) ([]*PromptCapsule, error) {
	query := `SELECT id, name, description, intent, domain, risk, status, source_url, source_type, retrieved_at, license_status, created_at, updated_at FROM prompt_capsules WHERE 1=1`
	args := []any{}
	if intent != "" {
		query += " AND intent LIKE ?"
		args = append(args, "%"+intent+"%")
	}
	if domain != "" {
		query += " AND domain = ?"
		args = append(args, domain)
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, string(status))
	}
	query += " ORDER BY created_at DESC"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list capsules: %w", err)
	}
	defer rows.Close()
	result := make([]*PromptCapsule, 0)
	for rows.Next() {
		var c PromptCapsule
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.Intent, &c.Domain, &c.Risk, &c.Status, &c.SourceURL, &c.SourceType, &c.RetrievedAt, &c.LicenseStatus, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan capsule: %w", err)
		}
		result = append(result, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list capsules: %w", err)
	}
	return result, nil
}

func (r *sqliteRepository) UpdateCapsule(ctx context.Context, c *PromptCapsule) error {
	c.UpdatedAt = timeNow()
	result, err := r.db.ExecContext(ctx,
		`UPDATE prompt_capsules SET name=?, description=?, intent=?, domain=?, risk=?, status=?, source_url=?, source_type=?, retrieved_at=?, license_status=?, updated_at=? WHERE id = ?`,
		c.Name, c.Description, c.Intent, c.Domain, c.Risk, c.Status, c.SourceURL, c.SourceType, c.RetrievedAt, c.LicenseStatus, c.UpdatedAt, c.ID)
	if err != nil {
		return fmt.Errorf("update capsule: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update capsule rows affected: %w", err)
	}
	if n == 0 {
		return ErrCapsuleNotFound
	}
	return nil
}

func (r *sqliteRepository) UpdateCapsuleStatus(ctx context.Context, id string, status CapsuleStatus) error {
	result, err := r.db.ExecContext(ctx, `UPDATE prompt_capsules SET status = ?, updated_at = ? WHERE id = ?`, string(status), timeNow(), id)
	if err != nil {
		return fmt.Errorf("update capsule status: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update capsule status rows affected: %w", err)
	}
	if n == 0 {
		return ErrCapsuleNotFound
	}
	return nil
}

func (r *sqliteRepository) CreateVersion(ctx context.Context, v PromptVersion) error {
	now := timeNow()
	if v.CreatedAt == 0 {
		v.CreatedAt = now
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO prompt_capsule_versions (id, version, content, content_hash, token_estimate, placement, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.Version, v.Content, v.ContentHash, v.TokenEstimate, v.Placement, v.CreatedAt)
	return err
}

func (r *sqliteRepository) GetVersion(ctx context.Context, capsuleID, version string) (*PromptVersion, error) {
	query := `SELECT id, version, content, content_hash, token_estimate, placement, created_at FROM prompt_capsule_versions WHERE id = ?`
	args := []any{capsuleID}
	if version != "" {
		query += " AND version = ?"
		args = append(args, version)
	}
	query += " ORDER BY created_at DESC LIMIT 1"
	var v PromptVersion
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&v.ID, &v.Version, &v.Content, &v.ContentHash, &v.TokenEstimate, &v.Placement, &v.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}
	return &v, nil
}

func (r *sqliteRepository) GetVersions(ctx context.Context, capsuleID string) ([]PromptVersion, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, version, content, content_hash, token_estimate, placement, created_at FROM prompt_capsule_versions WHERE id = ? ORDER BY created_at DESC`,
		capsuleID)
	if err != nil {
		return nil, fmt.Errorf("get versions: %w", err)
	}
	defer rows.Close()
	result := make([]PromptVersion, 0)
	for rows.Next() {
		var v PromptVersion
		if err := rows.Scan(&v.ID, &v.Version, &v.Content, &v.ContentHash, &v.TokenEstimate, &v.Placement, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		result = append(result, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get versions: %w", err)
	}
	return result, nil
}

func (r *sqliteRepository) GetLatestVersion(ctx context.Context, capsuleID string) (*PromptVersion, error) {
	return r.GetVersion(ctx, capsuleID, "")
}

func (r *sqliteRepository) FindVersionByHash(ctx context.Context, contentHash string) (*PromptVersion, error) {
	var v PromptVersion
	err := r.db.QueryRowContext(ctx,
		`SELECT id, version, content, content_hash, token_estimate, placement, created_at FROM prompt_capsule_versions WHERE content_hash = ? ORDER BY created_at DESC LIMIT 1`,
		contentHash).Scan(&v.ID, &v.Version, &v.Content, &v.ContentHash, &v.TokenEstimate, &v.Placement, &v.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find version by hash: %w", err)
	}
	return &v, nil
}

func (r *sqliteRepository) CreateTrace(ctx context.Context, t PromptTrace) error {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	if t.CreatedAt == 0 {
		t.CreatedAt = timeNow()
	}
	confidenceVal := sql.NullFloat64{Float64: t.Confidence, Valid: true}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO prompt_route_traces (id, request_id, capsule_id, capsule_version, decision, confidence, reason_code, latency_ms, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.RequestID, t.CapsuleID, t.CapsuleVersion, t.Decision, confidenceVal, t.ReasonCode, t.LatencyMs, t.CreatedAt)
	if err != nil {
		return fmt.Errorf("create trace: %w", err)
	}
	return nil
}

func (r *sqliteRepository) CreateFeedback(ctx context.Context, f PromptFeedback) error {
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	if f.CreatedAt == 0 {
		f.CreatedAt = timeNow()
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO prompt_feedback (id, request_id, capsule_id, capsule_version, outcome, user_override, added_tokens, provider_error_code, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.RequestID, f.CapsuleID, f.CapsuleVersion, f.Outcome, f.UserOverride, f.AddedTokens, f.ProviderErrorCode, f.CreatedAt)
	if err != nil {
		return fmt.Errorf("create feedback: %w", err)
	}
	return nil
}

func (r *sqliteRepository) ListFeedback(ctx context.Context, requestID string) ([]PromptFeedback, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, request_id, capsule_id, capsule_version, outcome, user_override, added_tokens, provider_error_code, created_at FROM prompt_feedback WHERE request_id = ? ORDER BY created_at DESC`,
		requestID)
	if err != nil {
		return nil, fmt.Errorf("list feedback: %w", err)
	}
	defer rows.Close()
	result := make([]PromptFeedback, 0)
	for rows.Next() {
		var f PromptFeedback
		if err := rows.Scan(&f.ID, &f.RequestID, &f.CapsuleID, &f.CapsuleVersion, &f.Outcome, &f.UserOverride, &f.AddedTokens, &f.ProviderErrorCode, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan feedback: %w", err)
		}
		result = append(result, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list feedback: %w", err)
	}
	return result, nil
}

func (r *sqliteRepository) CreateEvaluation(ctx context.Context, e PromptEvaluation) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.CreatedAt == 0 {
		e.CreatedAt = timeNow()
	}
	confidenceVal := sql.NullFloat64{Float64: e.Confidence, Valid: true}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO prompt_evaluations (id, dataset_case, expected_capsule, actual_capsule, decision, confidence, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.DatasetCase, e.ExpectedCapsule, e.ActualCapsule, e.Decision, confidenceVal, e.CreatedAt)
	if err != nil {
		return fmt.Errorf("create evaluation: %w", err)
	}
	return nil
}

// Preflight implements ports.PromptPort — deterministic-first retrieval.
// Returns (nil, nil) when no capsules match so callers can distinguish from errors.
func (r *sqliteRepository) Preflight(ctx context.Context, intent, domain string) (*ports.PromptEnvelope, error) {
	capsules, err := r.ListCapsules(ctx, intent, domain, CapsuleStatusActive, 2)
	if err != nil {
		return nil, err
	}
	if len(capsules) == 0 {
		return nil, nil
	}
	return &ports.PromptEnvelope{
		ID:     capsules[0].ID,
		Name:   capsules[0].Name,
		Domain: capsules[0].Domain,
		Intent: splitIntent(capsules[0].Intent),
	}, nil
}

// PreflightWithVersions performs preflight with version info for token budget.
func (r *sqliteRepository) PreflightWithVersions(ctx context.Context, intent, domain string, maxCapsules int, opts FilterOptions) ([]*PromptCapsule, error) {
	capsules, err := r.ListCapsules(ctx, intent, domain, CapsuleStatusActive, maxCapsules)
	if err != nil {
		return nil, err
	}

	versions := make(map[string]*PromptVersion)
	for _, c := range capsules {
		v, err := r.GetLatestVersion(ctx, c.ID)
		if err == nil && v != nil {
			versions[c.ID] = v
		}
	}

	return FilterWithVersions(ctx, capsules, versions, opts)
}

// RecordFeedback implements ports.PromptPort.
// The note is hashed before persistence to comply with the security constraint
// that no raw user text is stored in structured columns.
func (r *sqliteRepository) RecordFeedback(ctx context.Context, promptID string, rating int, note string) error {
	noteHash := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(note)))
	fb := PromptFeedback{
		ID:                uuid.NewString(),
		RequestID:         promptID,
		CapsuleID:         promptID,
		Outcome:           fmt.Sprintf("rating:%d", rating),
		UserOverride:      rating < 3,
		ProviderErrorCode: noteHash,
		CreatedAt:         timeNow(),
	}
	return r.CreateFeedback(ctx, fb)
}

func (r *sqliteRepository) GetMetrics(ctx context.Context) (*Metrics, error) {
	var m Metrics
	err := r.db.QueryRowContext(ctx,
		`SELECT (SELECT COUNT(*) FROM prompt_capsules), (SELECT COUNT(*) FROM prompt_capsule_versions), (SELECT COUNT(*) FROM prompt_route_traces), (SELECT COUNT(*) FROM prompt_feedback), (SELECT COUNT(*) FROM prompt_evaluations), (SELECT AVG(confidence) FROM prompt_route_traces WHERE confidence IS NOT NULL)`,
	).Scan(&m.TotalCapsules, &m.TotalVersions, &m.TotalTraces, &m.TotalFeedback, &m.TotalEvaluations, &m.AvgConfidence)
	if err != nil {
		return nil, fmt.Errorf("get metrics: %w", err)
	}
	return &m, nil
}
