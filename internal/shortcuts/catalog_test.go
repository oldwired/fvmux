package shortcuts

import (
	"reflect"
	"testing"
)

func TestCatalogueScopesAndChordsAreUnique(t *testing.T) {
	knownScopes := map[string]bool{}
	for _, scope := range Scopes {
		if knownScopes[scope] {
			t.Fatalf("duplicate scope %q", scope)
		}
		knownScopes[scope] = true
		seen := map[string]bool{}
		for _, binding := range ForScope(scope) {
			if binding.Chord == "" || binding.Action == "" {
				t.Errorf("scope %q has incomplete descriptor: %+v", scope, binding)
			}
			if seen[binding.Chord] {
				t.Errorf("scope %q repeats chord %q", scope, binding.Chord)
			}
			seen[binding.Chord] = true
		}
	}
	for _, binding := range Catalogue {
		if !knownScopes[binding.Scope] {
			t.Errorf("binding has unlisted scope %q", binding.Scope)
		}
	}
}

func TestFilesCatalogueCoversImplementedKeys(t *testing.T) {
	var got []string
	for _, binding := range ForScope(ScopeFiles) {
		got = append(got, binding.Chord)
	}
	want := []string{"Tab", "Enter", "F5", "F6", "F7", "F8", "Ctrl-R", "Delete", "Esc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Files key catalogue = %v, want implementation keys %v", got, want)
	}
	if compact := Compact(ScopeFiles); compact != "F5 cp · F6 mv · F7 mkdir · F8 del · Ctrl-R refresh" {
		t.Fatalf("Files compact hint = %q", compact)
	}
}
