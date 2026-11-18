package octavia

import "context"

// PoolRepository provides read operations for pools.
type PoolRepository struct{ queries *Queries }

func NewPoolRepository(db DBTX) *PoolRepository { return &PoolRepository{queries: New(db)} }
func (r *PoolRepository) GetAll(ctx context.Context) ([]PoolGetAllRow, error) {
	return r.queries.PoolGetAll(ctx)
}

// LoadBalancerRepository provides read operations for load balancers.
type LoadBalancerRepository struct{ queries *Queries }

func NewLoadBalancerRepository(db DBTX) *LoadBalancerRepository {
	return &LoadBalancerRepository{queries: New(db)}
}
func (r *LoadBalancerRepository) GetAllWithVIP(ctx context.Context) ([]LoadBalancerGetAllWithVIPRow, error) {
	return r.queries.LoadBalancerGetAllWithVIP(ctx)
}

// AmphoraRepository provides read operations for amphorae.
type AmphoraRepository struct{ queries *Queries }

func NewAmphoraRepository(db DBTX) *AmphoraRepository { return &AmphoraRepository{queries: New(db)} }
func (r *AmphoraRepository) GetAll(ctx context.Context) ([]AmphoraGetAllRow, error) {
	return r.queries.AmphoraGetAll(ctx)
}

// QuotaRepository provides read operations for project quotas.
type QuotaRepository struct{ queries *Queries }

func NewQuotaRepository(db DBTX) *QuotaRepository { return &QuotaRepository{queries: New(db)} }

// GetAll returns per-project limits and Octavia's cached in_use_* counters,
// which are what Octavia enforces quota against.
func (r *QuotaRepository) GetAll(ctx context.Context) ([]Quota, error) {
	return r.queries.QuotaGetAll(ctx)
}

// GetUsage rebuilds project counts from non-DELETED objects, independently
// of the cached in_use_* counters. It includes usage-only projects. It scans
// every quota resource table; compare it with GetAll to detect counter drift.
func (r *QuotaRepository) GetUsage(ctx context.Context) ([]QuotaUsageGetAllRow, error) {
	return r.queries.QuotaUsageGetAll(ctx)
}
