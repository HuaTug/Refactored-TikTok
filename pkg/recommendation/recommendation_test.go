package recommendation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// =====================================================
// CTR Client Tests
// =====================================================

func TestNewCTRServiceClient(t *testing.T) {
	config := &CTRServiceConfig{
		ServiceURL:      "http://localhost:8000",
		Timeout:         200 * time.Millisecond,
		MaxRetries:      2,
		RetryDelay:      50 * time.Millisecond,
		DefaultModel:    "deepfm",
		MaxIdleConns:    100,
		MaxConnsPerHost: 50,
	}

	client := NewCTRServiceClient(config)

	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	if client.config.ServiceURL != config.ServiceURL {
		t.Errorf("Expected ServiceURL %s, got %s", config.ServiceURL, client.config.ServiceURL)
	}

	if client.config.DefaultModel != "deepfm" {
		t.Errorf("Expected DefaultModel 'deepfm', got %s", client.config.DefaultModel)
	}
}

func TestDefaultCTRServiceConfig(t *testing.T) {
	config := DefaultCTRServiceConfig()

	if config == nil {
		t.Fatal("Expected non-nil config")
	}

	if config.ServiceURL != "http://localhost:8000" {
		t.Errorf("Expected default ServiceURL 'http://localhost:8000', got %s", config.ServiceURL)
	}

	if config.MaxRetries != 2 {
		t.Errorf("Expected default MaxRetries 2, got %d", config.MaxRetries)
	}

	if config.DefaultModel != "deepfm" {
		t.Errorf("Expected default model 'deepfm', got %s", config.DefaultModel)
	}
}

func TestCTRClientPredict_MockServer(t *testing.T) {
	// Create mock CTR server
	mockResponse := CTRPredictResponse{
		Predictions: []CTRPrediction{
			{VideoID: 1, Score: 0.85, CTR: 0.85, IsFinish: 0.7, IsLike: 0.5},
			{VideoID: 2, Score: 0.72, CTR: 0.72, IsFinish: 0.6, IsLike: 0.4},
			{VideoID: 3, Score: 0.65, CTR: 0.65, IsFinish: 0.5, IsLike: 0.3},
		},
		LatencyMs: 15.5,
		Model:     "deepfm",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
			return
		}

		if r.URL.Path == "/predict" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockResponse)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	// Create client with mock server URL
	config := &CTRServiceConfig{
		ServiceURL:      server.URL,
		Timeout:         2 * time.Second,
		MaxRetries:      1,
		DefaultModel:    "deepfm",
		MaxIdleConns:    10,
		MaxConnsPerHost: 5,
	}

	client := NewCTRServiceClient(config)
	client.setHealthy(true) // Force healthy status for test

	// Test predict
	ctx := context.Background()
	predictions, err := client.Predict(ctx, 123, []int64{1, 2, 3}, nil)

	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}

	if len(predictions) != 3 {
		t.Errorf("Expected 3 predictions, got %d", len(predictions))
	}

	if predictions[0].VideoID != 1 {
		t.Errorf("Expected first video ID 1, got %d", predictions[0].VideoID)
	}

	if predictions[0].CTR != 0.85 {
		t.Errorf("Expected first CTR 0.85, got %f", predictions[0].CTR)
	}
}

func TestCTRClientFallbackScoring(t *testing.T) {
	config := &CTRServiceConfig{
		ServiceURL: "http://invalid-url:9999", // Invalid URL
		Timeout:    100 * time.Millisecond,
	}

	client := NewCTRServiceClient(config)
	client.setHealthy(false) // Force unhealthy

	ctx := context.Background()
	predictions, err := client.Predict(ctx, 123, []int64{1, 2, 3}, nil)

	if err != nil {
		t.Fatalf("Fallback should not return error: %v", err)
	}

	if len(predictions) != 3 {
		t.Errorf("Expected 3 fallback predictions, got %d", len(predictions))
	}

	// Fallback should return default score 0.5
	for _, pred := range predictions {
		if pred.Score != 0.5 {
			t.Errorf("Expected fallback score 0.5, got %f", pred.Score)
		}
	}
}

func TestCTRClientEmptyVideoIDs(t *testing.T) {
	client := NewCTRServiceClient(nil)

	ctx := context.Background()
	predictions, err := client.Predict(ctx, 123, []int64{}, nil)

	if err != nil {
		t.Fatalf("Empty video IDs should not return error: %v", err)
	}

	if predictions != nil && len(predictions) != 0 {
		t.Errorf("Expected nil or empty predictions for empty input")
	}
}

// =====================================================
// Prediction Utility Tests
// =====================================================

func TestSortByScore(t *testing.T) {
	predictions := []CTRPrediction{
		{VideoID: 1, Score: 0.5},
		{VideoID: 2, Score: 0.9},
		{VideoID: 3, Score: 0.3},
		{VideoID: 4, Score: 0.7},
	}

	sorted := SortByScore(predictions)

	// Should be in descending order
	expectedOrder := []int64{2, 4, 1, 3}
	for i, pred := range sorted {
		if pred.VideoID != expectedOrder[i] {
			t.Errorf("Position %d: expected video %d, got %d", i, expectedOrder[i], pred.VideoID)
		}
	}
}

func TestGetTopN(t *testing.T) {
	predictions := []CTRPrediction{
		{VideoID: 1, Score: 0.5},
		{VideoID: 2, Score: 0.9},
		{VideoID: 3, Score: 0.3},
		{VideoID: 4, Score: 0.7},
	}

	// Get top 2
	top2 := GetTopN(predictions, 2)
	if len(top2) != 2 {
		t.Errorf("Expected 2 items, got %d", len(top2))
	}
	if top2[0] != 2 || top2[1] != 4 {
		t.Errorf("Expected [2, 4], got %v", top2)
	}

	// Get more than available
	topAll := GetTopN(predictions, 10)
	if len(topAll) != 4 {
		t.Errorf("Expected 4 items (all), got %d", len(topAll))
	}
}

func TestPredictionToScoredVideo(t *testing.T) {
	pred := CTRPrediction{
		VideoID:  123,
		Score:    0.85,
		CTR:      0.85,
		IsFinish: 0.75,
		IsLike:   0.6,
		IsShare:  0.3,
	}

	scored := PredictionToScoredVideo(pred)

	if scored.VideoID != 123 {
		t.Errorf("Expected VideoID 123, got %d", scored.VideoID)
	}

	if scored.Score != 0.85 {
		t.Errorf("Expected Score 0.85, got %f", scored.Score)
	}

	if scored.Features["ctr"] != 0.85 {
		t.Errorf("Expected ctr feature 0.85, got %f", scored.Features["ctr"])
	}

	if scored.Features["is_finish"] != 0.75 {
		t.Errorf("Expected is_finish feature 0.75, got %f", scored.Features["is_finish"])
	}
}

func TestGenerateReasons(t *testing.T) {
	// High CTR
	pred1 := CTRPrediction{CTR: 0.9, IsFinish: 0.5, IsLike: 0.3}
	reasons1 := generateReasons(pred1)
	if len(reasons1) == 0 || reasons1[0] != "高点击率内容" {
		t.Errorf("Expected '高点击率内容' reason for high CTR")
	}

	// High finish rate
	pred2 := CTRPrediction{CTR: 0.5, IsFinish: 0.8, IsLike: 0.3}
	reasons2 := generateReasons(pred2)
	found := false
	for _, r := range reasons2 {
		if r == "完播率高" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected '完播率高' reason for high finish rate")
	}

	// High like rate
	pred3 := CTRPrediction{CTR: 0.5, IsFinish: 0.5, IsLike: 0.6}
	reasons3 := generateReasons(pred3)
	found = false
	for _, r := range reasons3 {
		if r == "用户喜爱" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected '用户喜爱' reason for high like rate")
	}
}

// =====================================================
// Integrated Engine Tests
// =====================================================

func TestDefaultIntegratedConfig(t *testing.T) {
	config := DefaultIntegratedConfig()

	if config == nil {
		t.Fatal("Expected non-nil config")
	}

	if config.MaxRecallCandidates != 500 {
		t.Errorf("Expected MaxRecallCandidates 500, got %d", config.MaxRecallCandidates)
	}

	if config.EnableCTRRanking != true {
		t.Error("Expected EnableCTRRanking to be true by default")
	}

	if config.DiversityLambda != 0.7 {
		t.Errorf("Expected DiversityLambda 0.7, got %f", config.DiversityLambda)
	}

	if config.ExplorationRatio != 0.1 {
		t.Errorf("Expected ExplorationRatio 0.1, got %f", config.ExplorationRatio)
	}
}

func TestNewIntegratedRecommendationEngine(t *testing.T) {
	config := &IntegratedRecommendConfig{
		MaxRecallCandidates: 100,
		EnableCTRRanking:    false,
		DiversityLambda:     0.5,
	}

	engine := NewIntegratedRecommendationEngine(config, nil, nil, nil)

	if engine == nil {
		t.Fatal("Expected non-nil engine")
	}

	if engine.config.MaxRecallCandidates != 100 {
		t.Errorf("Expected MaxRecallCandidates 100, got %d", engine.config.MaxRecallCandidates)
	}

	// CTR client should be nil when disabled
	if engine.ctrClient != nil {
		t.Error("Expected nil CTR client when EnableCTRRanking is false")
	}
}

func TestIntegratedEngine_SortByScore(t *testing.T) {
	engine := NewIntegratedRecommendationEngine(nil, nil, nil, nil)

	videos := []ScoredVideo{
		{VideoID: 1, Score: 0.3},
		{VideoID: 2, Score: 0.9},
		{VideoID: 3, Score: 0.5},
		{VideoID: 4, Score: 0.7},
	}

	sorted := engine.sortByScore(videos)

	expectedOrder := []int64{2, 4, 3, 1}
	for i, v := range sorted {
		if v.VideoID != expectedOrder[i] {
			t.Errorf("Position %d: expected video %d, got %d", i, expectedOrder[i], v.VideoID)
		}
	}
}

func TestIntegratedEngine_ConvertToScoredVideos(t *testing.T) {
	engine := NewIntegratedRecommendationEngine(nil, nil, nil, nil)

	videoIDs := []int64{1, 2, 3, 4, 5}
	scored := engine.convertToScoredVideos(videoIDs)

	if len(scored) != 5 {
		t.Errorf("Expected 5 videos, got %d", len(scored))
	}

	// First video should have highest score
	if scored[0].VideoID != 1 {
		t.Errorf("Expected first VideoID 1, got %d", scored[0].VideoID)
	}

	// Scores should be in descending order (preserving original order)
	for i := 1; i < len(scored); i++ {
		if scored[i].Score >= scored[i-1].Score {
			t.Errorf("Scores should be in descending order")
		}
	}
}

func TestIntegratedEngine_RerankMMR(t *testing.T) {
	engine := NewIntegratedRecommendationEngine(&IntegratedRecommendConfig{
		DiversityLambda: 0.7,
	}, nil, nil, nil)

	videos := []ScoredVideo{
		{VideoID: 1, Score: 0.9, Features: map[string]float64{"cat": 1.0}},
		{VideoID: 2, Score: 0.85, Features: map[string]float64{"cat": 1.0}}, // Same category as 1
		{VideoID: 3, Score: 0.8, Features: map[string]float64{"cat": 0.0}},  // Different category
		{VideoID: 4, Score: 0.75, Features: map[string]float64{"cat": 0.5}},
	}

	reranked := engine.rerankMMR(videos, 3, 0.7)

	if len(reranked) != 3 {
		t.Errorf("Expected 3 videos, got %d", len(reranked))
	}

	// First should still be highest score
	if reranked[0].VideoID != 1 {
		t.Errorf("First video should be VideoID 1 (highest score), got %d", reranked[0].VideoID)
	}
}

func TestIntegratedEngine_CalculateSimilarity(t *testing.T) {
	engine := NewIntegratedRecommendationEngine(nil, nil, nil, nil)

	// Test identical vectors
	v1 := ScoredVideo{Features: map[string]float64{"a": 1.0, "b": 0.0}}
	v2 := ScoredVideo{Features: map[string]float64{"a": 1.0, "b": 0.0}}
	sim := engine.calculateSimilarity(v1, v2)
	if sim < 0.99 {
		t.Errorf("Expected similarity ~1.0 for identical vectors, got %f", sim)
	}

	// Test orthogonal vectors
	v3 := ScoredVideo{Features: map[string]float64{"a": 1.0, "b": 0.0}}
	v4 := ScoredVideo{Features: map[string]float64{"a": 0.0, "b": 1.0}}
	sim2 := engine.calculateSimilarity(v3, v4)
	if sim2 > 0.01 {
		t.Errorf("Expected similarity ~0 for orthogonal vectors, got %f", sim2)
	}

	// Test empty features
	v5 := ScoredVideo{Features: map[string]float64{}}
	v6 := ScoredVideo{Features: map[string]float64{"a": 1.0}}
	sim3 := engine.calculateSimilarity(v5, v6)
	if sim3 != 0 {
		t.Errorf("Expected similarity 0 for empty features, got %f", sim3)
	}
}

func TestSqrt(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{4.0, 2.0},
		{9.0, 3.0},
		{16.0, 4.0},
		{0.0, 0.0},
		{-1.0, 0.0}, // Negative should return 0
	}

	for _, tc := range tests {
		result := sqrt(tc.input)
		diff := result - tc.expected
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.0001 {
			t.Errorf("sqrt(%f) = %f, expected %f", tc.input, result, tc.expected)
		}
	}
}

// =====================================================
// Integration Tests
// =====================================================

func TestIntegratedEngine_Recommend_NoRecallEngine(t *testing.T) {
	engine := NewIntegratedRecommendationEngine(&IntegratedRecommendConfig{
		EnableCTRRanking: false,
	}, nil, nil, nil)

	ctx := context.Background()
	resp, err := engine.Recommend(ctx, &RecommendRequest{
		UserID:    123,
		Limit:     10,
		RequestID: "test-req-001",
	})

	if err != nil {
		t.Fatalf("Recommend should not fail with nil recall engine: %v", err)
	}

	if resp == nil {
		t.Fatal("Expected non-nil response")
	}

	if len(resp.Videos) != 0 {
		t.Errorf("Expected 0 videos with nil recall engine, got %d", len(resp.Videos))
	}

	if resp.RequestID != "test-req-001" {
		t.Errorf("Expected RequestID 'test-req-001', got %s", resp.RequestID)
	}
}

// =====================================================
// Benchmark Tests
// =====================================================

func BenchmarkSortByScore(b *testing.B) {
	predictions := make([]CTRPrediction, 100)
	for i := 0; i < 100; i++ {
		predictions[i] = CTRPrediction{
			VideoID: int64(i),
			Score:   float64(i%10) / 10.0,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Make a copy to avoid sorting already sorted slice
		cp := make([]CTRPrediction, len(predictions))
		copy(cp, predictions)
		SortByScore(cp)
	}
}

func BenchmarkMMRReranking(b *testing.B) {
	engine := NewIntegratedRecommendationEngine(nil, nil, nil, nil)

	videos := make([]ScoredVideo, 50)
	for i := 0; i < 50; i++ {
		videos[i] = ScoredVideo{
			VideoID: int64(i),
			Score:   float64(50-i) / 50.0,
			Features: map[string]float64{
				"cat1": float64(i % 5),
				"cat2": float64(i % 3),
			},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cp := make([]ScoredVideo, len(videos))
		copy(cp, videos)
		engine.rerankMMR(cp, 20, 0.7)
	}
}

func BenchmarkCalculateSimilarity(b *testing.B) {
	engine := NewIntegratedRecommendationEngine(nil, nil, nil, nil)

	v1 := ScoredVideo{Features: map[string]float64{"a": 0.5, "b": 0.3, "c": 0.8, "d": 0.1}}
	v2 := ScoredVideo{Features: map[string]float64{"a": 0.4, "b": 0.6, "c": 0.2, "d": 0.9}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.calculateSimilarity(v1, v2)
	}
}
