// Package conformance runs the v0 conformance corpus.
//
// The corpus is data, not Go tests, so an implementation in any language can
// run it and be checked against the same expectations. The goal it exists to
// serve: two independent implementations agree byte-for-byte on canonical
// persisted values and result-for-result on evaluation.
//
// Two things the format deliberately cannot express, and which live in Go tests
// beside the kernel instead:
//
//   - Snapshot consistency (§40.14) needs a world held across a later
//     transaction, and a case here is one linear program.
//   - Resource limits (§35) are deployment policy, not language constants, so
//     no expectation about them would be portable.
//
// Claim identities and world sequence numbers are local to one store. The
// corpus only relies on claims being numbered from 1 in assertion order.
package conformance

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/felixubl/loom"
)

//go:embed corpus.json
var corpusJSON []byte

// Corpus is the whole file.
type Corpus struct {
	Version string `json:"version"`
	Cases   []Case `json:"cases"`
}

// Case is one program and everything expected of it.
//
// Each case runs in a fresh store. Program is Loom source. Output is the lines
// every statement renders, concatenated in order. Held is the canonical form of
// every value held when the program finishes, sorted by byte order. Error, when
// present, is the semantic error code the program must fail with, and Output
// then covers only the statements that ran before it.
type Case struct {
	Name    string   `json:"name"`
	Spec    string   `json:"spec,omitempty"`
	Note    string   `json:"note,omitempty"`
	Program string   `json:"program"`
	Output  []string `json:"output"`
	Held    []string `json:"held"`
	Error   string   `json:"error,omitempty"`
}

// Load returns the embedded corpus.
func Load() (*Corpus, error) {
	var c Corpus
	if err := json.Unmarshal(corpusJSON, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Failure is one way a case did not conform.
type Failure struct {
	Case   string
	Detail string
}

func (f Failure) Error() string { return f.Case + ": " + f.Detail }

// Run executes a case against a fresh store and returns everything that did not
// match. An empty slice means the case conforms.
func Run(c Case) []Failure {
	var fail []Failure
	add := func(format string, args ...any) {
		fail = append(fail, Failure{Case: c.Name, Detail: fmt.Sprintf(format, args...)})
	}

	store := loom.New()
	session := loom.NewSession(store)

	program, err := store.ParseProgram(c.Program)
	if err != nil {
		if c.Error == "" {
			add("parse: %v", err)
		} else if c.Error != "syntax_error" {
			add("parse failed with %v, expected %s", err, c.Error)
		}
		return fail
	}
	if c.Error == "syntax_error" {
		add("parsed, expected a syntax error")
		return fail
	}

	results, runErr := session.RunProgram(program)

	var output []string
	for _, r := range results {
		output = append(output, loom.Render(store, r)...)
	}
	if diff := compare("output", c.Output, output); diff != "" {
		add("%s", diff)
	}

	switch {
	case c.Error == "" && runErr != nil:
		add("run: %v", runErr)
	case c.Error != "" && runErr == nil:
		add("expected error %s, program succeeded", c.Error)
	case c.Error != "" && runErr != nil:
		var semantic *loom.Error
		if !errors.As(runErr, &semantic) {
			add("expected error %s, got %v", c.Error, runErr)
		} else if string(semantic.Code) != c.Error {
			add("expected error %s, got %s (%v)", c.Error, semantic.Code, runErr)
		}
	}

	held := store.World().Held()
	if diff := compare("held", c.Held, held); diff != "" {
		add("%s", diff)
	}

	// Every persisted value must survive being written out in surface syntax
	// and read back: Source and Parse are inverses over persistable values.
	for _, id := range store.World().Values() {
		source := store.Source(id)
		term, err := loom.Parse(source)
		if err != nil {
			add("round trip: %s does not parse: %v", source, err)
			continue
		}
		v, err := store.World().Eval(term)
		if err != nil {
			add("round trip: %s does not evaluate: %v", source, err)
			continue
		}
		back, err := loom.Persist(v)
		if err != nil {
			add("round trip: %s is not persistable: %v", source, err)
			continue
		}
		if back != id {
			add("round trip: %s came back as %s", store.Canonical(id), store.Canonical(back))
		}
	}
	return fail
}

func compare(what string, want, got []string) string {
	if len(want) == 0 && len(got) == 0 {
		return ""
	}
	if len(want) == len(got) {
		same := true
		for i := range want {
			if want[i] != got[i] {
				same = false
				break
			}
		}
		if same {
			return ""
		}
	}
	return fmt.Sprintf("%s mismatch\n  want %#v\n  got  %#v", what, want, got)
}
