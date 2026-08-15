package conformance_test

import (
	"testing"

	"github.com/felixubl/loom/conformance"
)

func TestCorpus(t *testing.T) {
	corpus, err := conformance.Load()
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	if len(corpus.Cases) == 0 {
		t.Fatal("corpus is empty")
	}

	seen := map[string]bool{}
	for _, c := range corpus.Cases {
		if seen[c.Name] {
			t.Errorf("duplicate case name %q", c.Name)
		}
		seen[c.Name] = true

		t.Run(c.Name, func(t *testing.T) {
			for _, f := range conformance.Run(c) {
				t.Error(f.Detail)
			}
		})
	}
}
