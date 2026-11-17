package manila

import "context"

// QuotaRepository reads Manila's persisted quota accounting. Project usage sums
// users without adding the duplicate share-type accounting. User overrides are
// returned separately and must never replace project or share-type limits.
type QuotaRepository struct{ queries *Queries }

func NewQuotaRepository(db DBTX) *QuotaRepository { return &QuotaRepository{queries: New(db)} }

func (r *QuotaRepository) GetProjectLimits(ctx context.Context) ([]QuotaGetProjectLimitsRow, error) {
	return r.queries.QuotaGetProjectLimits(ctx)
}

func (r *QuotaRepository) GetProjectUsage(ctx context.Context) ([]QuotaUsageGetProjectTotalsRow, error) {
	return r.queries.QuotaUsageGetProjectTotals(ctx)
}

func (r *QuotaRepository) GetShareTypeLimits(ctx context.Context) ([]QuotaGetShareTypeLimitsRow, error) {
	return r.queries.QuotaGetShareTypeLimits(ctx)
}

func (r *QuotaRepository) GetShareTypeUsage(ctx context.Context) ([]QuotaUsageGetShareTypeTotalsRow, error) {
	return r.queries.QuotaUsageGetShareTypeTotals(ctx)
}

func (r *QuotaRepository) GetDefaultLimits(ctx context.Context) ([]QuotaGetDefaultLimitsRow, error) {
	return r.queries.QuotaGetDefaultLimits(ctx)
}

func (r *QuotaRepository) GetUserLimits(ctx context.Context) ([]QuotaGetUserLimitsRow, error) {
	return r.queries.QuotaGetUserLimits(ctx)
}
