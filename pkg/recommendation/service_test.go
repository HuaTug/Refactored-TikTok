package recommendation

import (
	"testing"
	"time"
)

// =====================================================
// Hot Score Service Tests
// =====================================================

func TestNewHotScoreConfig(t *testing.T) {
	config := &HotScoreConfig{
		TimeWindows: []TimeWindow{
			{Name: "1h", Duration: time.Hour},
			{Name: "24h", Duration: 24 * time.Hour},
		},
		Weights: InteractionWeights{
			View:     1.0,
			Like:     3.0,
			Comment:  5.0,
			Share:    8.0,
			Favorite: 4.0,
		},
		DecayHalfLife:    6 * time.Hour,
		QualityThreshold: 0.5,
	}

	if len(config.TimeWindows) != 2 {
		t.Errorf("Expected 2 time windows, got %d", len(config.TimeWindows))
	}

	if config.Weights.Share != 8.0 {
		t.Errorf("Expected Share weight 8.0, got %f", config.Weights.Share)
	}
}

func TestCalculateBaseScore(t *testing.T) {
	weights := InteractionWeights{
		View:     1.0,
		Like:     3.0,
		Comment:  5.0,
		Share:    8.0,
		Favorite: 4.0,
	}

	stats := VideoStats{
		ViewCount:     1000,
		LikeCount:     100,
		CommentCount:  20,
		ShareCount:    10,
		FavoriteCount: 50,
	}

	// BaseScore = Views×1 + Likes×3 + Comments×5 + Shares×8 + Favorites×4
	// = 1000×1 + 100×3 + 20×5 + 10×8 + 50×4
	// = 1000 + 300 + 100 + 80 + 200 = 1680
	expectedScore := float64(1000*1 + 100*3 + 20*5 + 10*8 + 50*4)

	actualScore := CalculateBaseScore(stats, weights)

	if actualScore != expectedScore {
		t.Errorf("Expected base score %f, got %f", expectedScore, actualScore)
	}
}

func TestCalculateDecay(t *testing.T) {
	halfLife := 6 * time.Hour

	// At t=0, decay should be 1.0
	decay0 := CalculateDecay(0, halfLife)
	if decay0 < 0.99 || decay0 > 1.01 {
		t.Errorf("At t=0, decay should be ~1.0, got %f", decay0)
	}

	// At t=halfLife, decay should be 0.5
	decayHalf := CalculateDecay(halfLife, halfLife)
	if decayHalf < 0.49 || decayHalf > 0.51 {
		t.Errorf("At t=halfLife, decay should be ~0.5, got %f", decayHalf)
	}

	// At t=2*halfLife, decay should be 0.25
	decayDouble := CalculateDecay(2*halfLife, halfLife)
	if decayDouble < 0.24 || decayDouble > 0.26 {
		t.Errorf("At t=2*halfLife, decay should be ~0.25, got %f", decayDouble)
	}
}

func TestCalculateHotScore(t *testing.T) {
	config := &HotScoreConfig{
		Weights: InteractionWeights{
			View:     1.0,
			Like:     3.0,
			Comment:  5.0,
			Share:    8.0,
			Favorite: 4.0,
		},
		DecayHalfLife:      6 * time.Hour,
		QualityThreshold:   0.5,
		QualityBonusWeight: 0.2,
		BaseScoreWeight:    0.3,
		DeltaScoreWeight:   0.7,
	}

	current := VideoStats{
		ViewCount:     1000,
		LikeCount:     100,
		CommentCount:  20,
		ShareCount:    10,
		FavoriteCount: 50,
	}

	previous := VideoStats{
		ViewCount:     800,
		LikeCount:     80,
		CommentCount:  15,
		ShareCount:    8,
		FavoriteCount: 40,
	}

	publishTime := time.Now().Add(-3 * time.Hour)
	qualityScore := 0.8

	score := CalculateHotScore(config, current, previous, publishTime, qualityScore)

	// Score should be positive
	if score <= 0 {
		t.Errorf("Hot score should be positive, got %f", score)
	}

	// Score with higher quality should be higher
	scoreHighQuality := CalculateHotScore(config, current, previous, publishTime, 1.0)
	scoreLowQuality := CalculateHotScore(config, current, previous, publishTime, 0.3)

	if scoreHighQuality <= scoreLowQuality {
		t.Errorf("Higher quality should result in higher score")
	}
}

// =====================================================
// Video Stats Helper Types and Functions for Testing
// =====================================================

// VideoStats represents video statistics for hot score calculation
type VideoStats struct {
	ViewCount     int64
	LikeCount     int64
	CommentCount  int64
	ShareCount    int64
	FavoriteCount int64
}

// InteractionWeights defines weights for different interactions
type InteractionWeights struct {
	View     float64
	Like     float64
	Comment  float64
	Share    float64
	Favorite float64
}

// TimeWindow represents a time window configuration
type TimeWindow struct {
	Name     string
	Duration time.Duration
}

// HotScoreConfig configuration for hot score calculation
type HotScoreConfig struct {
	TimeWindows        []TimeWindow
	Weights            InteractionWeights
	DecayHalfLife      time.Duration
	QualityThreshold   float64
	QualityBonusWeight float64
	BaseScoreWeight    float64
	DeltaScoreWeight   float64
}

// CalculateBaseScore calculates the base interaction score
func CalculateBaseScore(stats VideoStats, weights InteractionWeights) float64 {
	return float64(stats.ViewCount)*weights.View +
		float64(stats.LikeCount)*weights.Like +
		float64(stats.CommentCount)*weights.Comment +
		float64(stats.ShareCount)*weights.Share +
		float64(stats.FavoriteCount)*weights.Favorite
}

// CalculateDecay calculates time decay factor
func CalculateDecay(elapsed time.Duration, halfLife time.Duration) float64 {
	if halfLife <= 0 {
		return 1.0
	}
	// Decay = e^(-λt) where λ = ln(2)/halfLife
	lambda := 0.693147 / float64(halfLife.Seconds()) // ln(2) ≈ 0.693147
	return expDecay(-lambda * float64(elapsed.Seconds()))
}

// expDecay simple exponential function
func expDecay(x float64) float64 {
	if x > 700 {
		return 1e308
	}
	if x < -700 {
		return 0
	}

	result := 1.0
	term := 1.0
	for i := 1; i <= 100; i++ {
		term *= x / float64(i)
		result += term
		if term < 1e-15 && term > -1e-15 {
			break
		}
	}
	return result
}

// CalculateHotScore calculates the complete hot score
func CalculateHotScore(config *HotScoreConfig, current, previous VideoStats, publishTime time.Time, qualityScore float64) float64 {
	// Calculate base scores
	currentBase := CalculateBaseScore(current, config.Weights)
	previousBase := CalculateBaseScore(previous, config.Weights)

	// Calculate delta (increment)
	delta := currentBase - previousBase
	if delta < 0 {
		delta = 0
	}

	// Calculate time decay
	elapsed := time.Since(publishTime)
	decay := CalculateDecay(elapsed, config.DecayHalfLife)

	// Calculate quality bonus
	qualityBonus := 1.0
	if qualityScore > config.QualityThreshold {
		qualityBonus = 1.0 + (qualityScore-config.QualityThreshold)*config.QualityBonusWeight
	}

	// Final score: weighted combination of decayed base and delta, with quality bonus
	score := (currentBase*config.BaseScoreWeight*decay + delta*config.DeltaScoreWeight) * qualityBonus

	return score
}

// =====================================================
// User Profile Service Tests
// =====================================================

func TestInterestTagUpdate(t *testing.T) {
	tags := make(map[string]float64)

	// Add a new tag
	updateInterestTag(tags, "搞笑", 0.5, 0.1)
	if tags["搞笑"] != 0.5 {
		t.Errorf("Expected tag weight 0.5, got %f", tags["搞笑"])
	}

	// Update existing tag (should blend)
	updateInterestTag(tags, "搞笑", 0.8, 0.3)
	expected := 0.5*(1-0.3) + 0.8*0.3 // 0.35 + 0.24 = 0.59
	diff := tags["搞笑"] - expected
	if diff < -0.01 || diff > 0.01 {
		t.Errorf("Expected blended weight ~%f, got %f", expected, tags["搞笑"])
	}
}

func TestInterestDecay(t *testing.T) {
	tags := map[string]float64{
		"搞笑": 1.0,
		"美食": 0.5,
		"科技": 0.2,
	}

	decayFactor := 0.9
	applyDecay(tags, decayFactor)

	if tags["搞笑"] != 0.9 {
		t.Errorf("Expected 搞笑 = 0.9 after decay, got %f", tags["搞笑"])
	}

	if tags["美食"] != 0.45 {
		t.Errorf("Expected 美食 = 0.45 after decay, got %f", tags["美食"])
	}

	if tags["科技"] != 0.18 {
		t.Errorf("Expected 科技 = 0.18 after decay, got %f", tags["科技"])
	}
}

func TestNormalizeWeights(t *testing.T) {
	weights := map[string]float64{
		"a": 2.0,
		"b": 3.0,
		"c": 5.0,
	}

	normalizeWeights(weights)

	// Sum should be 1.0
	sum := 0.0
	for _, w := range weights {
		sum += w
	}

	if sum < 0.99 || sum > 1.01 {
		t.Errorf("Normalized weights should sum to 1.0, got %f", sum)
	}

	// Check relative proportions
	if weights["a"] > weights["b"] || weights["b"] > weights["c"] {
		t.Error("Normalized weights should preserve relative order")
	}
}

func TestTopKTags(t *testing.T) {
	tags := map[string]float64{
		"a": 0.1,
		"b": 0.5,
		"c": 0.3,
		"d": 0.8,
		"e": 0.2,
	}

	top3 := getTopKTags(tags, 3)

	if len(top3) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(top3))
	}

	// Should be in order: d(0.8), b(0.5), c(0.3)
	expectedOrder := []string{"d", "b", "c"}
	for i, expected := range expectedOrder {
		if top3[i] != expected {
			t.Errorf("Position %d: expected %s, got %s", i, expected, top3[i])
		}
	}
}

// Helper functions for user profile tests

func updateInterestTag(tags map[string]float64, tag string, weight float64, blendRatio float64) {
	if current, exists := tags[tag]; exists {
		tags[tag] = current*(1-blendRatio) + weight*blendRatio
	} else {
		tags[tag] = weight
	}
}

func applyDecay(tags map[string]float64, factor float64) {
	for tag := range tags {
		tags[tag] *= factor
	}
}

func normalizeWeights(weights map[string]float64) {
	sum := 0.0
	for _, w := range weights {
		sum += w
	}
	if sum > 0 {
		for k := range weights {
			weights[k] /= sum
		}
	}
}

func getTopKTags(tags map[string]float64, k int) []string {
	// Simple selection sort for small k
	type tagWeight struct {
		tag    string
		weight float64
	}

	list := make([]tagWeight, 0, len(tags))
	for t, w := range tags {
		list = append(list, tagWeight{t, w})
	}

	// Sort descending
	for i := 0; i < len(list)-1; i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].weight > list[i].weight {
				list[i], list[j] = list[j], list[i]
			}
		}
	}

	result := make([]string, 0, k)
	for i := 0; i < k && i < len(list); i++ {
		result = append(result, list[i].tag)
	}
	return result
}

// =====================================================
// Benchmark Tests
// =====================================================

func BenchmarkCalculateDecay(b *testing.B) {
	halfLife := 6 * time.Hour
	elapsed := 3 * time.Hour

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateDecay(elapsed, halfLife)
	}
}

func BenchmarkCalculateBaseScore(b *testing.B) {
	weights := InteractionWeights{
		View: 1.0, Like: 3.0, Comment: 5.0, Share: 8.0, Favorite: 4.0,
	}
	stats := VideoStats{
		ViewCount: 10000, LikeCount: 1000, CommentCount: 100, ShareCount: 50, FavoriteCount: 200,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateBaseScore(stats, weights)
	}
}

func BenchmarkUpdateInterestTag(b *testing.B) {
	tags := make(map[string]float64)
	for i := 0; i < 100; i++ {
		tags[string(rune('A'+i%26))] = float64(i) / 100.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		updateInterestTag(tags, "测试", 0.5, 0.3)
	}
}

func BenchmarkGetTopKTags(b *testing.B) {
	tags := make(map[string]float64)
	for i := 0; i < 100; i++ {
		tags[string(rune('A'+i%26))+string(rune('0'+i%10))] = float64(i) / 100.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getTopKTags(tags, 10)
	}
}
