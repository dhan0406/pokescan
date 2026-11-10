package pokeapi

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/dhan0406/pokescan/backend/apierr"
)

func pikachuRoutes() map[string]string {
	return map[string]string{
		"/api/v2/pokemon-species/pikachu": "species_pikachu",
		"/api/v2/evolution-chain/10":      "evolution_chain_10",
		"/api/v2/pokemon/25":              "pokemon_pikachu",
	}
}

func node(name string, next ...EvolutionNode) EvolutionNode {
	return EvolutionNode{Name: name, EvolvesTo: next}
}

func TestSpecies_pikachu(t *testing.T) {
	f := newFake(t, pikachuRoutes(), nil)
	got, err := NewClient(f.URL, f.Client()).Species(context.Background(), "pikachu")
	if err != nil {
		t.Fatalf("Species: %v", err)
	}
	if got.ID != 25 || got.Name != "pikachu" {
		t.Errorf("ID, Name = %d, %q, want 25, pikachu", got.ID, got.Name)
	}
	if !reflect.DeepEqual(got.Types, []string{"electric"}) {
		t.Errorf("Types = %v, want [electric]", got.Types)
	}
	wantStats := map[string]int{
		"hp": 35, "attack": 55, "defense": 40,
		"special-attack": 50, "special-defense": 50, "speed": 90,
	}
	if !reflect.DeepEqual(got.Stats, wantStats) {
		t.Errorf("Stats = %v, want %v", got.Stats, wantStats)
	}
	wantChain := node("pichu", node("pikachu", node("raichu")))
	if !reflect.DeepEqual(got.Evolution, wantChain) {
		t.Errorf("Evolution = %+v, want %+v", got.Evolution, wantChain)
	}
}

func TestSpecies_branchingChain(t *testing.T) {
	f := newFake(t, map[string]string{
		"/api/v2/pokemon-species/eevee": "species_eevee",
		"/api/v2/evolution-chain/67":    "evolution_chain_67",
		"/api/v2/pokemon/133":           "pokemon_eevee",
	}, nil)
	got, err := NewClient(f.URL, f.Client()).Species(context.Background(), "eevee")
	if err != nil {
		t.Fatalf("Species: %v", err)
	}
	if got.Evolution.Name != "eevee" {
		t.Errorf("root = %q, want eevee", got.Evolution.Name)
	}
	var names []string
	for _, n := range got.Evolution.EvolvesTo {
		names = append(names, n.Name)
	}
	if len(names) != 8 {
		t.Errorf("eevee has %d evolutions %v, want 8", len(names), names)
	}
	found := false
	for _, n := range names {
		found = found || n == "vaporeon"
	}
	if !found {
		t.Errorf("vaporeon missing from %v", names)
	}
}

func TestSpecies_noEvolutions(t *testing.T) {
	f := newFake(t, map[string]string{
		"/api/v2/pokemon-species/tauros": "species_tauros",
		"/api/v2/evolution-chain/63":     "evolution_chain_63",
		"/api/v2/pokemon/128":            "pokemon_tauros",
	}, nil)
	got, err := NewClient(f.URL, f.Client()).Species(context.Background(), "tauros")
	if err != nil {
		t.Fatalf("Species: %v", err)
	}
	if got.Evolution.Name != "tauros" || len(got.Evolution.EvolvesTo) != 0 {
		t.Errorf("Evolution = %+v, want tauros with no evolutions", got.Evolution)
	}
}

func TestSpecies_flavorTextIsFirstEnglishNormalized(t *testing.T) {
	f := newFake(t, pikachuRoutes(), nil)
	got, err := NewClient(f.URL, f.Client()).Species(context.Background(), "pikachu")
	if err != nil {
		t.Fatalf("Species: %v", err)
	}
	want := "When several of these POKéMON gather, their electricity could build and cause lightning storms."
	if got.FlavorText != want {
		t.Errorf("FlavorText = %q, want %q", got.FlavorText, want)
	}
	if strings.ContainsAny(got.FlavorText, "\n\f") {
		t.Errorf("FlavorText still has line breaks: %q", got.FlavorText)
	}
}

func TestSpecies_noEnglishFlavorText(t *testing.T) {
	routes := pikachuRoutes()
	routes["/api/v2/pokemon-species/pikachu"] = "species_no_english"
	f := newFake(t, routes, nil)
	got, err := NewClient(f.URL, f.Client()).Species(context.Background(), "pikachu")
	if err != nil {
		t.Fatalf("Species: %v", err)
	}
	if got.FlavorText != "" {
		t.Errorf("FlavorText = %q, want empty", got.FlavorText)
	}
	if got.Name != "pikachu" {
		t.Errorf("rest of the species should still load, Name = %q", got.Name)
	}
}

func TestSpecies_followsChainIDNotAbsoluteURL(t *testing.T) {
	// The fixture's evolution_chain.url is https://pokeapi.co/...; the client
	// must request the chain from its own base URL instead.
	f := newFake(t, pikachuRoutes(), nil)
	if _, err := NewClient(f.URL, f.Client()).Species(context.Background(), "pikachu"); err != nil {
		t.Fatalf("Species: %v", err)
	}
	found := false
	for _, p := range f.paths {
		found = found || p == "/api/v2/evolution-chain/10"
	}
	if !found {
		t.Errorf("fake never saw the chain request; paths = %q", f.paths)
	}
}

func TestSpecies_usesThreeCalls(t *testing.T) {
	f := newFake(t, pikachuRoutes(), nil)
	if _, err := NewClient(f.URL, f.Client()).Species(context.Background(), "pikachu"); err != nil {
		t.Fatalf("Species: %v", err)
	}
	if len(f.paths) != 3 {
		t.Errorf("made %d requests %q, want 3", len(f.paths), f.paths)
	}
}

func TestSpecies_anyCallFailingFailsAll(t *testing.T) {
	for p := range pikachuRoutes() {
		t.Run("500 on "+p, func(t *testing.T) {
			f := newFake(t, pikachuRoutes(), map[string]int{p: 500})
			_, err := NewClient(f.URL, f.Client()).Species(context.Background(), "pikachu")
			if !errors.Is(err, apierr.ErrUpstream) {
				t.Errorf("err = %v, want ErrUpstream", err)
			}
		})
	}
	t.Run("404 species", func(t *testing.T) {
		f := newFake(t, nil, nil)
		_, err := NewClient(f.URL, f.Client()).Species(context.Background(), "missingno")
		if !errors.Is(err, apierr.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
}

func TestSpecies_lowercasesAndEscapes(t *testing.T) {
	f := newFake(t, pikachuRoutes(), nil)
	c := NewClient(f.URL, f.Client())

	if _, err := c.Species(context.Background(), "Pikachu"); err != nil {
		t.Fatalf("Species(Pikachu): %v", err)
	}
	if len(f.paths) == 0 || f.paths[0] != "/api/v2/pokemon-species/pikachu" {
		t.Errorf("paths = %q, want first to be the lowercase species path", f.paths)
	}

	f.paths = nil
	if _, err := c.Species(context.Background(), "../type/fire"); err == nil {
		t.Error("Species(../type/fire) succeeded, want an error")
	}
	for _, p := range f.paths {
		if !strings.HasPrefix(p, "/api/v2/pokemon-species/") || strings.Contains(p, "/../") {
			t.Errorf("request escaped the species endpoint: %q", p)
		}
	}
}

func TestSpecies_badChainURLIsUpstream(t *testing.T) {
	routes := pikachuRoutes()
	routes["/api/v2/pokemon-species/pikachu"] = "species_bad_chain_url"
	f := newFake(t, routes, nil)
	_, err := NewClient(f.URL, f.Client()).Species(context.Background(), "pikachu")
	if !errors.Is(err, apierr.ErrUpstream) {
		t.Errorf("err = %v, want ErrUpstream", err)
	}
}
