package metrics

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

type pgxpoolStatsCollector struct {
	pool *pgxpool.Pool

	acquireCount            *prometheus.Desc
	acquireDuration         *prometheus.Desc
	acquiredConns           *prometheus.Desc
	canceledAcquireCount    *prometheus.Desc
	constructingConns       *prometheus.Desc
	emptyAcquireCount       *prometheus.Desc
	idleConns               *prometheus.Desc
	totalConns              *prometheus.Desc
	maxConns                *prometheus.Desc
	newConnsCount           *prometheus.Desc
	maxLifetimeDestroyCount *prometheus.Desc
	maxIdleDestroyCount     *prometheus.Desc
}

func RegisterPgxpoolStatsCollector(pool *pgxpool.Pool) {
	prometheus.MustRegister(newPgxpoolStatsCollector(pool))
}

func newPgxpoolStatsCollector(pool *pgxpool.Pool) prometheus.Collector {
	prefix := func(name string) string {
		return "pgxpool_" + name
	}
	return &pgxpoolStatsCollector{
		pool: pool,
		acquireCount: prometheus.NewDesc(
			prefix("acquire_count"),
			"The cumulative count of successful acquires from the pool.",
			nil, nil,
		),
		acquireDuration: prometheus.NewDesc(
			prefix("acquired_duration"),
			"The total duration of all successful acquires from the pool.",
			nil, nil,
		),
		acquiredConns: prometheus.NewDesc(
			prefix("acquired_conns"),
			"The number of currently acquired connections in the pool.",
			nil, nil,
		),
		canceledAcquireCount: prometheus.NewDesc(
			prefix("canceled_acquire_count"),
			"The cumulative count of acquires from the pool that were canceled by a context.",
			nil, nil,
		),
		constructingConns: prometheus.NewDesc(
			prefix("constructing_conns"),
			"The number of conns with construction in progress in the pool.",
			nil, nil,
		),
		emptyAcquireCount: prometheus.NewDesc(
			prefix("empty_acquire_count"),
			"The cumulative count of successful acquires from the pool that waited for a resource to be released or constructed because the pool was empty.",
			nil, nil,
		),
		idleConns: prometheus.NewDesc(
			prefix("idle_conns"),
			"The number of currently idle conns in the pool.",
			nil, nil,
		),
		totalConns: prometheus.NewDesc(
			prefix("total_conns"),
			"The total number of resources currently in the pool. The value is the sum of ConstructingConns, AcquiredConns, and IdleConns.",
			nil, nil,
		),
		maxConns: prometheus.NewDesc(
			prefix("max_conns"),
			"The maximum size of the pool.",
			nil, nil,
		),
		newConnsCount: prometheus.NewDesc(
			prefix("new_conns_count"),
			"The cumulative count of new connections opened.",
			nil, nil,
		),
		maxLifetimeDestroyCount: prometheus.NewDesc(
			prefix("max_lifetime_destroy_count"),
			"The cumulative count of connections destroyed because they exceeded MaxConnLifetime.",
			nil, nil,
		),
		maxIdleDestroyCount: prometheus.NewDesc(
			prefix("max_idle_destroy_count"),
			"The cumulative count of connections destroyed because they exceeded MaxConnIdleTime.",
			nil, nil,
		),
	}
}

func (c *pgxpoolStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.acquireCount
	ch <- c.acquireDuration
	ch <- c.acquiredConns
	ch <- c.canceledAcquireCount
	ch <- c.constructingConns
	ch <- c.emptyAcquireCount
	ch <- c.idleConns
	ch <- c.totalConns
	ch <- c.maxConns
	ch <- c.newConnsCount
	ch <- c.maxLifetimeDestroyCount
	ch <- c.maxIdleDestroyCount
}

func (c *pgxpoolStatsCollector) Collect(ch chan<- prometheus.Metric) {
	stats := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.acquireCount, prometheus.GaugeValue, float64(stats.AcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.acquireDuration, prometheus.GaugeValue, float64(stats.AcquireDuration().Seconds()))
	ch <- prometheus.MustNewConstMetric(c.acquiredConns, prometheus.GaugeValue, float64(stats.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.canceledAcquireCount, prometheus.GaugeValue, float64(stats.CanceledAcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.constructingConns, prometheus.GaugeValue, float64(stats.ConstructingConns()))
	ch <- prometheus.MustNewConstMetric(c.emptyAcquireCount, prometheus.GaugeValue, float64(stats.EmptyAcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.idleConns, prometheus.GaugeValue, float64(stats.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.totalConns, prometheus.GaugeValue, float64(stats.TotalConns()))
	ch <- prometheus.MustNewConstMetric(c.maxConns, prometheus.GaugeValue, float64(stats.MaxConns()))
	ch <- prometheus.MustNewConstMetric(c.newConnsCount, prometheus.GaugeValue, float64(stats.NewConnsCount()))
	ch <- prometheus.MustNewConstMetric(c.maxLifetimeDestroyCount, prometheus.GaugeValue, float64(stats.MaxLifetimeDestroyCount()))
	ch <- prometheus.MustNewConstMetric(c.maxIdleDestroyCount, prometheus.GaugeValue, float64(stats.MaxIdleDestroyCount()))
}
