package store

import (
	"context"
	"testing"
)

func TestUpsertProfileThenGet(t *testing.T) {
	st, cleanup := testStore(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	name := "Test User"
	age := 30
	height := 175.5
	weight := 70.2
	p := Profile{Name: &name, Age: &age, HeightCm: &height, WeightKg: &weight}
	if err := st.UpsertProfile(ctx, p); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := st.GetProfile(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name == nil || *got.Name != name {
		t.Fatalf("name=%v, want %s", got.Name, name)
	}
	if got.Age == nil || *got.Age != age {
		t.Fatalf("age=%v, want %d", got.Age, age)
	}
}
