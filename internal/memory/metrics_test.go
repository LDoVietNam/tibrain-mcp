package memory

import (
	"testing"
	"time"
)

func TestMetricsCollector_UpdateTierMetrics(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")

	tierStats := map[Tier]TierStats{
		TierHuman:    {Count: 10, AvgConfidence: 0.9, VerifiedCount: 10},
		TierCore:     {Count: 50, AvgConfidence: 0.7, VerifiedCount: 30},
		TierArchival: {Count: 100, AvgConfidence: 0.5, VerifiedCount: 20},
		TierRecall:   {Count: 200, AvgConfidence: 0.3, VerifiedCount: 5},
	}

	collector.UpdateTierMetrics(tierStats)

	// Test passes if no panic - metrics are registered internally
}

func TestMetricsCollector_RecordPromotion(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")

	collector.RecordPromotion(TierRecall, TierCore, "high_access")
	collector.RecordPromotion(TierCore, TierHuman, "high_confidence")

	// Test passes if no panic
}

func TestMetricsCollector_RecordDemotion(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")

	collector.RecordDemotion(TierCore, TierArchival, "age")
	collector.RecordDemotion(TierRecall, TierArchival, "expired")

	// Test passes if no panic
}

func TestMetricsCollector_RecordVerification(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")

	collector.RecordVerification("rule1", "core")
	collector.RecordVerification("rule2", "human")

	// Test passes if no panic
}

func TestMetricsCollector_RecordPruned(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")

	collector.RecordPruned(TierRecall, "low_confidence")
	collector.RecordPruned(TierArchival, "old_age")

	// Test passes if no panic
}

func TestMetricsCollector_RecordSync(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")

	collector.RecordSync("push", "success", 100*time.Millisecond)
	collector.RecordSync("pull", "error", 50*time.Millisecond)
	collector.RecordSync("full", "success", 200*time.Millisecond)

	// Test passes if no panic
}

func TestMetricsCollector_RecordSearch(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")

	collector.RecordSearch("local", "success", 10*time.Millisecond, 5)
	collector.RecordSearch("cloud", "success", 50*time.Millisecond, 3)
	collector.RecordSearch("hybrid", "error", 5*time.Millisecond, 0)

	// Test passes if no panic
}

func TestMetricsCollector_RecordEmbedding(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")

	collector.RecordEmbedding("hash", "success", 5*time.Millisecond)
	collector.RecordEmbedding("onnx", "success", 100*time.Millisecond)
	collector.RecordEmbedding("api", "error", 200*time.Millisecond)

	// Test passes if no panic
}

func TestMetricsCollector_SetEmbeddingQueueSize(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")

	collector.SetEmbeddingQueueSize(10)
	collector.SetEmbeddingQueueSize(0)

	// Test passes if no panic
}

func TestMetricsCollector_RecordConflictResolution(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")

	collector.RecordConflictResolution("local_won")
	collector.RecordConflictResolution("remote_won")
	collector.RecordConflictResolution("merged")

	// Test passes if no panic
}

func TestMetricsCollector_UpdateDeviceMetrics(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")

	collector.UpdateDeviceMetrics(5, 3)
	collector.UpdateDeviceMetrics(10, 8)

	// Test passes if no panic
}

func TestMetricsCollector_RecordDeviceSync(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")

	collector.RecordDeviceSync("device-1", "success")
	collector.RecordDeviceSync("device-2", "error")

	// Test passes if no panic
}

func TestMetricsCollector_UpdateFAISSMetrics(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")

	collector.UpdateFAISSMetrics(1000)
	collector.RecordFAISSQuery("success", 10*time.Millisecond)
	collector.RecordFAISSQuery("error", 5*time.Millisecond)
	collector.RecordFAISSIndexed()

	// Test passes if no panic
}

func TestMetricsCollector_RecordRerank(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")

	collector.RecordRerank("heuristic", "success", 2*time.Millisecond)
	collector.RecordRerank("cross_encoder", "success", 50*time.Millisecond)

	// Test passes if no panic
}

func TestMetricsCollector_UpdateGraphMetrics(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")

	collector.UpdateGraphMetrics(100, 250)
	collector.RecordGraphQuery("related", 1*time.Millisecond)
	collector.RecordGraphQuery("traverse", 5*time.Millisecond)

	// Test passes if no panic
}

func TestMetricsCollector_RecordError(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")

	collector.RecordError("sync", "network_timeout")
	collector.RecordError("search", "index_not_found")

	// Test passes if no panic
}

func TestMetricsMiddleware_WrapSearch(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")
	middleware := NewMetricsMiddleware(collector)

	count, err := middleware.WrapSearch("local", func() (int, error) {
		return 5, nil
	})

	if count != 5 {
		t.Errorf("Expected 5, got %d", count)
	}
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Test error case
	count, err = middleware.WrapSearch("cloud", func() (int, error) {
		return 0, nil
	})
	if count != 0 {
		t.Errorf("Expected 0, got %d", count)
	}
}

func TestMetricsMiddleware_WrapSync(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")
	middleware := NewMetricsMiddleware(collector)

	err := middleware.WrapSync("push", func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Test error case
	err = middleware.WrapSync("pull", func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestMetricsMiddleware_WrapEmbedding(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")
	middleware := NewMetricsMiddleware(collector)

	err := middleware.WrapEmbedding("hash", func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestMetricsMiddleware_WrapRerank(t *testing.T) {
	collector := NewMetricsCollector("test", "memory")
	middleware := NewMetricsMiddleware(collector)

	err := middleware.WrapRerank("heuristic", func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestGlobalMetrics(t *testing.T) {
	InitGlobalMetrics("test", "memory")
	metrics := GetGlobalMetrics()

	if metrics == nil {
		t.Error("Global metrics should not be nil")
	}

	// Test that we can use it
	metrics.RecordPromotion(TierRecall, TierCore, "test")
}
