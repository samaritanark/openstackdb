package cinder

import "context"

// QuotaRepository reads Cinder's persisted DbQuotaDriver accounting. Callers
// supply a deadline. Aggregate and name-suffixed volume-type resources are
// independent measurements; duplicates must not be combined as user totals.
type QuotaRepository struct{ queries *Queries }

func NewQuotaRepository(db DBTX) *QuotaRepository { return &QuotaRepository{queries: New(db)} }

func (r *QuotaRepository) GetProjectLimits(ctx context.Context) ([]QuotaGetActiveLimitsRow, error) {
	return r.queries.QuotaGetActiveLimits(ctx)
}

func (r *QuotaRepository) GetProjectUsage(ctx context.Context) ([]QuotaGetActiveUsageRow, error) {
	return r.queries.QuotaGetActiveUsage(ctx)
}

func (r *QuotaRepository) GetDefaultLimits(ctx context.Context) ([]QuotaGetDefaultClassLimitsRow, error) {
	return r.queries.QuotaGetDefaultClassLimits(ctx)
}

func (r *QuotaRepository) GetVolumeTypes(ctx context.Context) ([]VolumeTypeGetAllCurrentRow, error) {
	return r.queries.VolumeTypeGetAllCurrent(ctx)
}
