package layout

import (
	"math/rand"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/session"
)

// TestUnmarshalMalformedNeverPanics pins review finding #6: parseLeaf used
// to advance p.pos past len(src) on a leaf missing its '=', so the next
// slice op panicked with index-out-of-range instead of erroring. Every
// malformed input below must return a parse error and, above all, must NOT
// panic (each case runs under a recover guard that fails loudly if it does).
func TestUnmarshalMalformedNeverPanics(t *testing.T) {
	spawn := func(LeafSpec) (*session.Pane, error) {
		return &session.Pane{ID: session.NewPaneID()}, nil
	}
	cases := []struct {
		name string
		src  string
	}{
		{"leaf key without eq", "leaf:profile"},
		{"leaf short ident without eq", "leaf:x"},
		{"leaf empty body", "leaf:"},
		{"leaf bare comma value", "leaf:profile=a,b"}, // parses "a", then key "b" has no '='
		{"leaf trailing comma no key", "leaf:profile=a,"},
		{"split ratio no brace", "split-v:0.5"},
		{"split open brace then eof", "split-v:0.5{"},
		{"split truncated child key", "split-v:0.5{leaf:profile"},
		{"split first child only", "split-h:0.5{leaf:profile=x}"},
		{"split empty leaf child", "split-h:0.5{leaf:}{leaf:profile=x}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				n   *PaneNode
				err error
			)
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("Unmarshal(%q) panicked: %v", tc.src, r)
					}
				}()
				n, err = Unmarshal(tc.src, spawn)
			}()
			if err == nil {
				t.Fatalf("Unmarshal(%q) = %+v, want a parse error", tc.src, n)
			}
		})
	}
}

// TestMarshalQuotesDelimiterProfiles pins review finding #7: a profile name
// containing a grammar delimiter (, { } " \) must be quoted on Marshal so it
// survives Unmarshal. Before the fix these characters were written bare and
// derailed the parser (or silently truncated the profile).
func TestMarshalQuotesDelimiterProfiles(t *testing.T) {
	for _, profile := range []string{"a,b", "web}dev", `back\slash`, `qu"ote`, "br{ace"} {
		var got LeafSpec
		spawn := func(spec LeafSpec) (*session.Pane, error) {
			got = spec
			return &session.Pane{ID: session.NewPaneID(), Profile: spec.Profile, Title: spec.Title}, nil
		}
		p := &session.Pane{ID: session.NewPaneID(), Profile: profile}
		s := Marshal(Leaf(p))
		root, err := Unmarshal(s, spawn)
		if err != nil {
			t.Fatalf("Unmarshal(%q) [profile %q]: %v", s, profile, err)
		}
		if got.Profile != profile {
			t.Errorf("profile round-trip: marshaled %q -> LeafSpec.Profile %q, want %q", s, got.Profile, profile)
		}
		if again := Marshal(root); again != s {
			t.Errorf("re-marshal drift for profile %q:\nfirst %q\nagain %q", profile, s, again)
		}
	}
}

// TestMarshalUnmarshalRoundTripProperty is the round-trip property test the
// review handoff for #7 asks for: build many random trees over a hostile
// alphabet (delimiters, spaces, unicode), Marshal them, Unmarshal recording
// each LeafSpec, then re-Marshal the reconstruction. The two marshaled
// strings must be byte-identical and the recorded LeafSpecs must match the
// originals' profiles/titles in in-order sequence. Seed is fixed (not
// time-based) so a failure replays deterministically.
func TestMarshalUnmarshalRoundTripProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(1))

	const trees = 200
	for i := 0; i < trees; i++ {
		tree := randTree(rng, 5)

		// Originals in in-order (the order Marshal writes and the parser
		// calls spawn), so the recorded specs line up index-for-index.
		var wantProfiles, wantTitles []string
		for _, l := range tree.CollectLeaves() {
			wantProfiles = append(wantProfiles, l.Pane.Profile)
			wantTitles = append(wantTitles, l.Pane.Title)
		}

		s1 := Marshal(tree)

		var specs []LeafSpec
		spawn := func(spec LeafSpec) (*session.Pane, error) {
			specs = append(specs, spec)
			return &session.Pane{ID: session.NewPaneID(), Profile: spec.Profile, Title: spec.Title}, nil
		}
		back, err := Unmarshal(s1, spawn)
		if err != nil {
			t.Fatalf("tree %d: Unmarshal(%q): %v", i, s1, err)
		}

		s2 := Marshal(back)
		if s1 != s2 {
			t.Fatalf("tree %d: round-trip drift:\nfirst %q\nagain %q", i, s1, s2)
		}

		if len(specs) != len(wantProfiles) {
			t.Fatalf("tree %d: %d leaves parsed, want %d (marshaled %q)", i, len(specs), len(wantProfiles), s1)
		}
		for j := range specs {
			if specs[j].Profile != wantProfiles[j] {
				t.Fatalf("tree %d leaf %d: profile %q, want %q (marshaled %q)", i, j, specs[j].Profile, wantProfiles[j], s1)
			}
			if specs[j].Title != wantTitles[j] {
				t.Fatalf("tree %d leaf %d: title %q, want %q (marshaled %q)", i, j, specs[j].Title, wantTitles[j], s1)
			}
		}
	}
}

// randTree builds a random PaneNode of depth <= maxDepth. Leaves carry a
// non-empty profile (empty would Marshal to "shell") and a possibly-empty
// title drawn from a hostile alphabet.
func randTree(rng *rand.Rand, maxDepth int) *PaneNode {
	if maxDepth <= 0 || rng.Intn(3) == 0 {
		p := &session.Pane{
			ID:      session.NewPaneID(),
			Profile: randToken(rng, 1, 8), // never empty
			Title:   randToken(rng, 0, 8), // may be empty
		}
		return Leaf(p)
	}
	orient := views.SplitVertical
	if rng.Intn(2) == 0 {
		orient = views.SplitHorizontal
	}
	a := randTree(rng, maxDepth-1)
	b := randTree(rng, maxDepth-1)
	n := Split(orient, a, b)
	n.Ratio = randRatio(rng)
	return n
}

// randRatio returns a value strictly inside (0,1) that 'g'-formats and
// re-parses exactly: either a common preset or a random value in [0.05,0.95).
func randRatio(rng *rand.Rand) float64 {
	if rng.Intn(2) == 0 {
		return []float64{0.25, 0.5, 0.75}[rng.Intn(3)]
	}
	return 0.05 + 0.9*rng.Float64()
}

// hostileRunes deliberately includes every grammar delimiter (, { } " \),
// spaces, plain letters/digits, and multibyte unicode so both the bare and
// quoted Marshal paths get exercised.
var hostileRunes = []rune{
	'a', 'b', 'c', 'X', 'Y', 'Z', '0', '9',
	' ', ',', '{', '}', '"', '\\',
	'=', '.', '-', '_',
	'h', 'é', 'l', 'o', '🏠', '世', '界',
}

func randToken(rng *rand.Rand, minLen, maxLen int) string {
	n := minLen
	if maxLen > minLen {
		n += rng.Intn(maxLen - minLen + 1)
	}
	rs := make([]rune, n)
	for i := range rs {
		rs[i] = hostileRunes[rng.Intn(len(hostileRunes))]
	}
	return string(rs)
}
