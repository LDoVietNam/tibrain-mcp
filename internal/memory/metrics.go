package memory

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// MetricsCollector collects and exposes Prometheus metrics for the memory system
type MetricsCollector struct {
	mu       sync.RWMutex
	registry *prometheus.Registry

	// Memory tier distribution
	tierDistribution *prometheus.GaugeVec

	// Entry counts per tier
	entriesTotal *prometheus.GaugeVec

	// Average confidence per tier
	avgConfidence *prometheus.GaugeVec

	// Verified entries per tier
	verifiedEntries *prometheus.GaugeVec

	// Promotion events
	promotionsTotal *prometheus.CounterVec

	// Demotion events
	demotionsTotal *prometheus.CounterVec

	// Verification events
	verificationsTotal *prometheus.CounterVec

	// Pruning events
	prunedEntries *prometheus.CounterVec

	// Sync operations
	syncOperations *prometheus.CounterVec
	syncDuration   *prometheus.HistogramVec

	// Search operations
	searchOperations *prometheus.CounterVec
	searchDuration   *prometheus.HistogramVec
	searchResults    *prometheus.HistogramVec

	// Embedding pipeline
	embeddingQueueSize prometheus.Gauge
	embeddingProcessed *prometheus.CounterVec
	embeddingDuration  *prometheus.HistogramVec

	// Conflict resolution
	conflictsResolved *prometheus.CounterVec

	// Device sync
	devicesRegistered prometheus.Gauge
	devicesTrusted    prometheus.Gauge
	devicesSynced     *prometheus.CounterVec

	// FAISS cloud RAG
	faissIndexSize     prometheus.Gauge
	faissQueryDuration *prometheus.HistogramVec
	faissIndexed       prometheus.Counter

	// Reranker
	rerankOperations *prometheus.CounterVec
	rerankDuration   *prometheus.HistogramVec

	// Graph operations
	graphNodes    prometheus.Gauge
	graphEdges    prometheus.Gauge
	graphQueries  prometheus.Counter
	graphDuration *prometheus.HistogramVec

	// Errors
	errorsTotal *prometheus.CounterVec
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(namespace, subsystem string) *MetricsCollector {
	m := &MetricsCollector{}
	m.registry = prometheus.NewRegistry()
	factory := promauto.With(m.registry)

	// Tier distribution (percentage)
	m.tierDistribution = factory.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "tier_distribution_percent",
		Help:      "Percentage of entries in each tier",
	}, []string{"tier"})

	// Total entries per tier
	m.entriesTotal = factory.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "entries_total",
		Help:      "Total number of entries per tier",
	}, []string{"tier"})

	// Average confidence per tier
	m.avgConfidence = factory.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "avg_confidence",
		Help:      "Average confidence score per tier",
	}, []string{"tier"})

	// Verified entries per tier
	m.verifiedEntries = factory.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "verified_entries",
		Help:      "Number of verified entries per tier",
	}, []string{"tier"})

	// Promotions
	m.promotionsTotal = factory.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "promotions_total",
		Help:      "Total number of promotion events",
	}, []string{"from_tier", "to_tier", "reason"})

	// Demotions
	m.demotionsTotal = factory.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "demotions_total",
		Help:      "Total number of demotion events",
	}, []string{"from_tier", "to_tier", "reason"})

	// Verifications
	m.verificationsTotal = factory.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "verifications_total",
		Help:      "Total number of auto-verification events",
	}, []string{"rule", "tier"})

	// Pruning
	m.prunedEntries = factory.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "pruned_entries_total",
		Help:      "Total number of pruned entries",
	}, []string{"tier", "reason"})

	// Sync operations
	m.syncOperations = factory.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "sync_operations_total",
		Help:      "Total number of sync operations",
	}, []string{"operation", "status"})

	m.syncDuration = factory.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "sync_duration_seconds",
		Help:      "Duration of sync operations",
		Buckets:   prometheus.DefBuckets,
	}, []string{"operation"})

	// Search operations
	m.searchOperations = factory.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "search_operations_total",
		Help:      "Total number of search operations",
	}, []string{"source", "status"})

	m.searchDuration = factory.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "search_duration_seconds",
		Help:      "Duration of search operations",
		Buckets:   prometheus.DefBuckets,
	}, []string{"source"})

	m.searchResults = factory.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "search_results_count",
		Help:      "Number of results returned by search",
		Buckets:   []float64{0, 1, 5, 10, 20, 50, 100},
	}, []string{"source"})

	// Embedding pipeline
	m.embeddingQueueSize = factory.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "embedding_queue_size",
		Help:      "Current size of embedding processing queue",
	})

	m.embeddingProcessed = factory.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "embedding_processed_total",
		Help:      "Total number of embeddings processed",
	}, []string{"provider", "status"})

	m.embeddingDuration = factory.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "embedding_duration_seconds",
		Help:      "Duration of embedding generation",
		Buckets:   prometheus.DefBuckets,
	}, []string{"provider"})

	// Conflict resolution
	m.conflictsResolved = factory.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "conflicts_resolved_total",
		Help:      "Total number of conflicts resolved",
	}, []string{"resolution"})

	// Device sync
	m.devicesRegistered = factory.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "devices_registered",
		Help:      "Number of registered devices",
	})

	m.devicesTrusted = factory.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "devices_trusted",
		Help:      "Number of trusted devices",
	})

	m.devicesSynced = factory.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "devices_synced_total",
		Help:      "Total number of device sync operations",
	}, []string{"device_id", "status"})

	// FAISS cloud RAG
	m.faissIndexSize = factory.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "faiss_index_size",
		Help:      "Number of vectors in FAISS index",
	})

	m.faissQueryDuration = factory.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "faiss_query_duration_seconds",
		Help:      "Duration of FAISS queries",
		Buckets:   prometheus.DefBuckets,
	}, []string{"status"})

	m.faissIndexed = factory.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "faiss_indexed_total",
		Help:      "Total number of entries indexed in FAISS",
	})

	// Reranker
	m.rerankOperations = factory.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "rerank_operations_total",
		Help:      "Total number of rerank operations",
	}, []string{"reranker", "status"})

	m.rerankDuration = factory.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "rerank_duration_seconds",
		Help:      "Duration of rerank operations",
		Buckets:   prometheus.DefBuckets,
	}, []string{"reranker"})

	// Graph
	m.graphNodes = factory.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "graph_nodes",
		Help:      "Number of nodes in memory graph",
	})

	m.graphEdges = factory.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "graph_edges",
		Help:      "Number of edges in memory graph",
	})

	m.graphQueries = factory.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "graph_queries_total",
		Help:      "Total number of graph queries",
	})

	m.graphDuration = factory.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "graph_query_duration_seconds",
		Help:      "Duration of graph queries",
		Buckets:   prometheus.DefBuckets,
	}, []string{"operation"})

	// Errors
	m.errorsTotal = factory.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "errors_total",
		Help:      "Total number of errors",
	}, []string{"component", "error_type"})

	return m
}

// UpdateTierMetrics updates tier distribution and entry metrics
func (m *MetricsCollector) UpdateTierMetrics(tierStats map[Tier]TierStats) {
	m.mu.Lock()
	defer m.mu.Unlock()

	totalEntries := 0
	for _, stats := range tierStats {
		totalEntries += stats.Count
	}

	for tier, stats := range tierStats {
		tierStr := string(tier)
		m.entriesTotal.WithLabelValues(tierStr).Set(float64(stats.Count))

		if totalEntries > 0 {
			m.tierDistribution.WithLabelValues(tierStr).Set(float64(stats.Count) / float64(totalEntries) * 100)
		}

		if stats.Count > 0 {
			m.avgConfidence.WithLabelValues(tierStr).Set(stats.AvgConfidence)
		}

		m.verifiedEntries.WithLabelValues(tierStr).Set(float64(stats.VerifiedCount))
	}
}

// TierStats holds statistics for a tier
type TierStats struct {
	Count         int
	AvgConfidence float64
	VerifiedCount int
}

// RecordPromotion records a promotion event
func (m *MetricsCollector) RecordPromotion(from, to Tier, reason string) {
	m.promotionsTotal.WithLabelValues(string(from), string(to), reason).Inc()
}

// RecordDemotion records a demotion event
func (m *MetricsCollector) RecordDemotion(from, to Tier, reason string) {
	m.demotionsTotal.WithLabelValues(string(from), string(to), reason).Inc()
}

// RecordVerification records an auto-verification event
func (m *MetricsCollector) RecordVerification(rule, tier string) {
	m.verificationsTotal.WithLabelValues(rule, tier).Inc()
}

// RecordPruned records a pruned entry
func (m *MetricsCollector) RecordPruned(tier Tier, reason string) {
	m.prunedEntries.WithLabelValues(string(tier), reason).Inc()
}

// RecordSync records a sync operation
func (m *MetricsCollector) RecordSync(operation, status string, duration time.Duration) {
	m.syncOperations.WithLabelValues(operation, status).Inc()
	m.syncDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordSearch records a search operation
func (m *MetricsCollector) RecordSearch(source, status string, duration time.Duration, resultCount int) {
	m.searchOperations.WithLabelValues(source, status).Inc()
	m.searchDuration.WithLabelValues(source).Observe(duration.Seconds())
	m.searchResults.WithLabelValues(source).Observe(float64(resultCount))
}

// RecordEmbedding records an embedding operation
func (m *MetricsCollector) RecordEmbedding(provider, status string, duration time.Duration) {
	m.embeddingProcessed.WithLabelValues(provider, status).Inc()
	m.embeddingDuration.WithLabelValues(provider).Observe(duration.Seconds())
}

// SetEmbeddingQueueSize sets the current embedding queue size
func (m *MetricsCollector) SetEmbeddingQueueSize(size int) {
	m.embeddingQueueSize.Set(float64(size))
}

// RecordConflictResolution records a conflict resolution
func (m *MetricsCollector) RecordConflictResolution(resolution string) {
	m.conflictsResolved.WithLabelValues(resolution).Inc()
}

// UpdateDeviceMetrics updates device registry metrics
func (m *MetricsCollector) UpdateDeviceMetrics(registered, trusted int) {
	m.devicesRegistered.Set(float64(registered))
	m.devicesTrusted.Set(float64(trusted))
}

// RecordDeviceSync records a device sync operation
func (m *MetricsCollector) RecordDeviceSync(deviceID, status string) {
	m.devicesSynced.WithLabelValues(deviceID, status).Inc()
}

// UpdateFAISSMetrics updates FAISS index metrics
func (m *MetricsCollector) UpdateFAISSMetrics(indexSize int) {
	m.faissIndexSize.Set(float64(indexSize))
}

// RecordFAISSQuery records a FAISS query
func (m *MetricsCollector) RecordFAISSQuery(status string, duration time.Duration) {
	m.faissQueryDuration.WithLabelValues(status).Observe(duration.Seconds())
}

// RecordFAISSIndexed increments the FAISS indexed counter
func (m *MetricsCollector) RecordFAISSIndexed() {
	m.faissIndexed.Inc()
}

// RecordRerank records a rerank operation
func (m *MetricsCollector) RecordRerank(reranker, status string, duration time.Duration) {
	m.rerankOperations.WithLabelValues(reranker, status).Inc()
	m.rerankDuration.WithLabelValues(reranker).Observe(duration.Seconds())
}

// UpdateGraphMetrics updates graph metrics
func (m *MetricsCollector) UpdateGraphMetrics(nodes, edges int) {
	m.graphNodes.Set(float64(nodes))
	m.graphEdges.Set(float64(edges))
}

// RecordGraphQuery records a graph query
func (m *MetricsCollector) RecordGraphQuery(operation string, duration time.Duration) {
	m.graphQueries.Inc()
	m.graphDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordError records an error
func (m *MetricsCollector) RecordError(component, errorType string) {
	m.errorsTotal.WithLabelValues(component, errorType).Inc()
}

// MetricsMiddleware provides a middleware for recording operation metrics
type MetricsMiddleware struct {
	collector *MetricsCollector
}

// NewMetricsMiddleware creates a new metrics middleware
func NewMetricsMiddleware(collector *MetricsCollector) *MetricsMiddleware {
	return &MetricsMiddleware{collector: collector}
}

// WrapSearch wraps a search function with metrics
func (mm *MetricsMiddleware) WrapSearch(source string, fn func() (int, error)) (int, error) {
	start := time.Now()
	count, err := fn()
	duration := time.Since(start)

	status := "success"
	if err != nil {
		status = "error"
		mm.collector.RecordError("search", err.Error())
	}

	mm.collector.RecordSearch(source, status, duration, count)
	return count, err
}

// WrapSync wraps a sync function with metrics
func (mm *MetricsMiddleware) WrapSync(operation string, fn func() error) error {
	start := time.Now()
	err := fn()
	duration := time.Since(start)

	status := "success"
	if err != nil {
		status = "error"
		mm.collector.RecordError("sync", err.Error())
	}

	mm.collector.RecordSync(operation, status, duration)
	return err
}

// WrapEmbedding wraps an embedding function with metrics
func (mm *MetricsMiddleware) WrapEmbedding(provider string, fn func() error) error {
	start := time.Now()
	err := fn()
	duration := time.Since(start)

	status := "success"
	if err != nil {
		status = "error"
		mm.collector.RecordError("embedding", err.Error())
	}

	mm.collector.RecordEmbedding(provider, status, duration)
	return err
}

// WrapRerank wraps a rerank function with metrics
func (mm *MetricsMiddleware) WrapRerank(reranker string, fn func() error) error {
	start := time.Now()
	err := fn()
	duration := time.Since(start)

	status := "success"
	if err != nil {
		status = "error"
		mm.collector.RecordError("rerank", err.Error())
	}

	mm.collector.RecordRerank(reranker, status, duration)
	return err
}

// Global metrics instance (initialized by main)
var GlobalMetrics *MetricsCollector

// InitGlobalMetrics initializes the global metrics collector
func InitGlobalMetrics(namespace, subsystem string) {
	GlobalMetrics = NewMetricsCollector(namespace, subsystem)
}

// GetGlobalMetrics returns the global metrics collector
func GetGlobalMetrics() *MetricsCollector {
	return GlobalMetrics
}
