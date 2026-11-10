package cache

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/dhan0406/pokescan/backend/pokeapi"
	"github.com/dhan0406/pokescan/backend/tcg"
)

// The real clients and the cached wrappers must all satisfy the same interfaces.
var (
	_ CardFinder    = (*tcg.Client)(nil)
	_ CardFinder    = (*CachedCards)(nil)
	_ SpeciesGetter = (*pokeapi.Client)(nil)
	_ SpeciesGetter = (*CachedSpecies)(nil)
)

type fakeCards struct {
	calls int
	cards []tcg.Card
	err   error
}

func (f *fakeCards) FindCards(ctx context.Context, name, number string) ([]tcg.Card, error) {
	f.calls++
	return f.cards, f.err
}

type fakeSpecies struct {
	calls   int
	species pokeapi.Species
	err     error
}

func (f *fakeSpecies) Species(ctx context.Context, idOrName string) (pokeapi.Species, error) {
	f.calls++
	return f.species, f.err
}

var pikachuCard = tcg.Card{ID: "base1-58", Name: "Pikachu", Number: "58"}

func TestCachedCards_secondCallServedFromCache(t *testing.T) {
	next := &fakeCards{cards: []tcg.Card{pikachuCard}}
	c := NewCards(next, time.Minute, newClock().Now)
	for i := 0; i < 2; i++ {
		got, err := c.FindCards(context.Background(), "Pikachu", "58")
		if err != nil || !reflect.DeepEqual(got, next.cards) {
			t.Fatalf("call %d = %v, %v", i, got, err)
		}
	}
	if next.calls != 1 {
		t.Errorf("upstream called %d times, want 1", next.calls)
	}
}

func TestCachedCards_callAfterTTLGoesUpstream(t *testing.T) {
	clk := newClock()
	next := &fakeCards{cards: []tcg.Card{pikachuCard}}
	c := NewCards(next, time.Minute, clk.Now)
	c.FindCards(context.Background(), "Pikachu", "58")
	clk.Advance(2 * time.Minute)
	c.FindCards(context.Background(), "Pikachu", "58")
	if next.calls != 2 {
		t.Errorf("upstream called %d times, want 2", next.calls)
	}
}

func TestCachedCards_errorsNotStored(t *testing.T) {
	boom := errors.New("boom")
	next := &fakeCards{err: boom}
	c := NewCards(next, time.Minute, newClock().Now)

	if _, err := c.FindCards(context.Background(), "Pikachu", "58"); !errors.Is(err, boom) {
		t.Fatalf("first err = %v, want boom", err)
	}
	next.err, next.cards = nil, []tcg.Card{pikachuCard}
	got, err := c.FindCards(context.Background(), "Pikachu", "58")
	if err != nil || !reflect.DeepEqual(got, next.cards) {
		t.Errorf("second call = %v, %v, want the success", got, err)
	}
	if next.calls != 2 {
		t.Errorf("upstream called %d times, want 2", next.calls)
	}
}

func TestCachedCards_emptyResultStoredAsSuccess(t *testing.T) {
	next := &fakeCards{cards: []tcg.Card{}}
	c := NewCards(next, time.Minute, newClock().Now)
	c.FindCards(context.Background(), "Pikachu", "999")
	c.FindCards(context.Background(), "Pikachu", "999")
	if next.calls != 1 {
		t.Errorf("upstream called %d times, want 1 (empty result is a success)", next.calls)
	}
}

func TestCachedCards_keyNormalization(t *testing.T) {
	tests := []struct {
		name       string
		a, b       [2]string
		wantShared bool
	}{
		{"case and leading zeros", [2]string{"Pikachu", "025"}, [2]string{"pikachu", "25"}, true},
		{"surrounding space", [2]string{" Pikachu ", "25"}, [2]string{"Pikachu", "25"}, true},
		{"letter prefix case is sent as typed, so is not shared", [2]string{"Zekrom", "TG05"}, [2]string{"Zekrom", "tg05"}, false},
		{"prefixed number is not the bare number", [2]string{"Pikachu", "TG05"}, [2]string{"Pikachu", "5"}, false},
		{"different names", [2]string{"Pikachu", "25"}, [2]string{"Raichu", "25"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := &fakeCards{cards: []tcg.Card{pikachuCard}}
			c := NewCards(next, time.Minute, newClock().Now)
			c.FindCards(context.Background(), tt.a[0], tt.a[1])
			c.FindCards(context.Background(), tt.b[0], tt.b[1])
			want := 2
			if tt.wantShared {
				want = 1
			}
			if next.calls != want {
				t.Errorf("upstream called %d times, want %d", next.calls, want)
			}
		})
	}
}

func TestCachedSpecies_secondCallServedFromCache(t *testing.T) {
	next := &fakeSpecies{species: pokeapi.Species{ID: 25, Name: "pikachu"}}
	c := NewSpecies(next, time.Minute, newClock().Now)
	for i := 0; i < 2; i++ {
		got, err := c.Species(context.Background(), "pikachu")
		if err != nil || got.ID != 25 {
			t.Fatalf("call %d = %+v, %v", i, got, err)
		}
	}
	if next.calls != 1 {
		t.Errorf("upstream called %d times, want 1", next.calls)
	}
}

func TestCachedSpecies_keyNormalization(t *testing.T) {
	next := &fakeSpecies{species: pokeapi.Species{ID: 25}}
	c := NewSpecies(next, time.Minute, newClock().Now)
	c.Species(context.Background(), "Pikachu")
	c.Species(context.Background(), "pikachu")
	if next.calls != 1 {
		t.Errorf("upstream called %d times, want 1", next.calls)
	}
}

func TestCachedSpecies_errorsNotStored(t *testing.T) {
	boom := errors.New("boom")
	next := &fakeSpecies{err: boom}
	c := NewSpecies(next, time.Minute, newClock().Now)

	if _, err := c.Species(context.Background(), "pikachu"); !errors.Is(err, boom) {
		t.Fatalf("first err = %v, want boom", err)
	}
	next.err, next.species = nil, pokeapi.Species{ID: 25}
	got, err := c.Species(context.Background(), "pikachu")
	if err != nil || got.ID != 25 {
		t.Errorf("second call = %+v, %v, want the success", got, err)
	}
	if next.calls != 2 {
		t.Errorf("upstream called %d times, want 2", next.calls)
	}
}
