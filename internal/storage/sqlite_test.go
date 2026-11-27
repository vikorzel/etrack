package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func TestSaveAndListMealsWithIsolationAndBounds(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	t1 := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 15, 13, 30, 0, 0, time.UTC)
	t3 := time.Date(2026, 11, 20, 20, 0, 0, 0, time.UTC)

	id1, err := store.SaveMeal(ctx, Meal{UserID: 100, RecordedAt: t1, SatietyBefore: 3, SatietyAfter: 8})
	if err != nil {
		t.Fatalf("SaveMeal t1: %v", err)
	}
	id2, err := store.SaveMeal(ctx, Meal{UserID: 100, RecordedAt: t2, SatietyBefore: 4, SatietyAfter: 7})
	if err != nil {
		t.Fatalf("SaveMeal t2: %v", err)
	}
	if _, err := store.SaveMeal(ctx, Meal{UserID: 100, RecordedAt: t3, SatietyBefore: 2, SatietyAfter: 9}); err != nil {
		t.Fatalf("SaveMeal t3: %v", err)
	}
	// Different user must stay isolated.
	if _, err := store.SaveMeal(ctx, Meal{UserID: 200, RecordedAt: t2, SatietyBefore: 5, SatietyAfter: 6}); err != nil {
		t.Fatalf("SaveMeal user 200: %v", err)
	}

	if id1 == 0 || id2 == 0 || id1 == id2 {
		t.Fatalf("unexpected generated IDs: id1=%d id2=%d", id1, id2)
	}

	allUser100, err := store.ListMeals(ctx, 100, nil, nil)
	if err != nil {
		t.Fatalf("ListMeals all: %v", err)
	}
	if len(allUser100) != 3 {
		t.Fatalf("len(allUser100) = %d, want 3", len(allUser100))
	}

	// Half-open [t1, t3): includes t1 and t2, excludes t3.
	bounded, err := store.ListMeals(ctx, 100, &t1, &t3)
	if err != nil {
		t.Fatalf("ListMeals bounded: %v", err)
	}
	if len(bounded) != 2 {
		t.Fatalf("len(bounded) = %d, want 2", len(bounded))
	}
	if !bounded[0].RecordedAt.Equal(t1) || !bounded[1].RecordedAt.Equal(t2) {
		t.Fatalf("unexpected bounded timestamps: %+v", bounded)
	}

	user200, err := store.ListMeals(ctx, 200, nil, nil)
	if err != nil {
		t.Fatalf("ListMeals user 200: %v", err)
	}
	if len(user200) != 1 || user200[0].SatietyBefore != 5 {
		t.Fatalf("unexpected user200 meals: %+v", user200)
	}
}

func TestSaveMealRejectsInvalidSatiety(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	cases := []Meal{
		{UserID: 0, SatietyBefore: 5, SatietyAfter: 7},
		{UserID: 1, SatietyBefore: 0, SatietyAfter: 7},
		{UserID: 1, SatietyBefore: 11, SatietyAfter: 7},
		{UserID: 1, SatietyBefore: 5, SatietyAfter: 0},
		{UserID: 1, SatietyBefore: 5, SatietyAfter: 11},
	}
	for i, tc := range cases {
		if _, err := store.SaveMeal(ctx, tc); err == nil {
			t.Errorf("case %d (%+v): expected validation error, got nil", i, tc)
		}
	}
}

func TestCountMealsIsolationAndValidation(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	if _, err := store.CountMeals(ctx, 0); err == nil {
		t.Fatal("expected error for user_id=0")
	}

	count, err := store.CountMeals(ctx, 100)
	if err != nil || count != 0 {
		t.Fatalf("CountMeals(100) = %d, %v; want 0, nil", count, err)
	}

	for i := 0; i < 3; i++ {
		if _, err := store.SaveMeal(ctx, Meal{UserID: 100, SatietyBefore: 3, SatietyAfter: 7}); err != nil {
			t.Fatalf("SaveMeal user 100 #%d: %v", i, err)
		}
	}
	if _, err := store.SaveMeal(ctx, Meal{UserID: 200, SatietyBefore: 4, SatietyAfter: 8}); err != nil {
		t.Fatalf("SaveMeal user 200: %v", err)
	}

	count100, err := store.CountMeals(ctx, 100)
	if err != nil || count100 != 3 {
		t.Fatalf("CountMeals(100) = %d, %v; want 3, nil", count100, err)
	}
	count200, err := store.CountMeals(ctx, 200)
	if err != nil || count200 != 1 {
		t.Fatalf("CountMeals(200) = %d, %v; want 1, nil", count200, err)
	}
}

func TestSaveAndListWeightsWithIsolationAndBounds(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	t1 := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 15, 13, 30, 0, 0, time.UTC)
	t3 := time.Date(2026, 11, 20, 20, 0, 0, 0, time.UTC)

	id1, err := store.SaveWeight(ctx, Weight{UserID: 100, RecordedAt: t1, WeightKg: 75.4})
	if err != nil {
		t.Fatalf("SaveWeight t1: %v", err)
	}
	id2, err := store.SaveWeight(ctx, Weight{UserID: 100, RecordedAt: t2, WeightKg: 74.8})
	if err != nil {
		t.Fatalf("SaveWeight t2: %v", err)
	}
	if _, err := store.SaveWeight(ctx, Weight{UserID: 100, RecordedAt: t3, WeightKg: 74.2}); err != nil {
		t.Fatalf("SaveWeight t3: %v", err)
	}
	if _, err := store.SaveWeight(ctx, Weight{UserID: 200, RecordedAt: t2, WeightKg: 62.0}); err != nil {
		t.Fatalf("SaveWeight user 200: %v", err)
	}

	if id1 == 0 || id2 == 0 || id1 == id2 {
		t.Fatalf("unexpected generated weight IDs: id1=%d id2=%d", id1, id2)
	}

	all100, err := store.ListWeights(ctx, 100, nil, nil)
	if err != nil {
		t.Fatalf("ListWeights all: %v", err)
	}
	if len(all100) != 3 || all100[0].WeightKg != 75.4 || all100[2].WeightKg != 74.2 {
		t.Fatalf("unexpected all100 weights: %+v", all100)
	}

	bounded, err := store.ListWeights(ctx, 100, &t1, &t3)
	if err != nil {
		t.Fatalf("ListWeights bounded: %v", err)
	}
	if len(bounded) != 2 || !bounded[0].RecordedAt.Equal(t1) || !bounded[1].RecordedAt.Equal(t2) {
		t.Fatalf("unexpected bounded weights: %+v", bounded)
	}

	user200, err := store.ListWeights(ctx, 200, nil, nil)
	if err != nil {
		t.Fatalf("ListWeights user 200: %v", err)
	}
	if len(user200) != 1 || user200[0].WeightKg != 62.0 {
		t.Fatalf("unexpected user200 weights: %+v", user200)
	}
}

func TestSaveWeightRejectsInvalidInput(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	cases := []Weight{
		{UserID: 0, WeightKg: 70},
		{UserID: 1, WeightKg: 0},
		{UserID: 1, WeightKg: -5},
		{UserID: 1, WeightKg: 500.1},
	}
	for i, tc := range cases {
		if _, err := store.SaveWeight(ctx, tc); err == nil {
			t.Errorf("case %d (%+v): expected validation error, got nil", i, tc)
		}
	}
}
