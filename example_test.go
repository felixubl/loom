package loom_test

import (
	"fmt"
	"log"

	"github.com/felixubl/loom"
)

func Example() {
	store := loom.New()

	// Writes are transaction commands, never ordinary expressions, so they
	// cannot be lost, duplicated or reordered by evaluation.
	tx := store.Begin()
	fact, err := loom.Parse("knows(Alice)(Bob)")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := tx.Assert(fact); err != nil {
		log.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}

	// Reads run against an immutable snapshot. Constructing a value and asking
	// whether it is asserted are separate operations.
	world := store.World()
	question, err := loom.Parse("holds(knows(Alice)(Bob))")
	if err != nil {
		log.Fatal(err)
	}
	answer, err := world.Eval(question)
	if err != nil {
		log.Fatal(err)
	}
	id, err := loom.Persist(answer)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(store.Source(id))

	// Patterns query held values structurally. A capture is not a lambda
	// variable: it belongs to the query interface, not the calculus.
	pattern, err := store.ParsePattern("knows(?who)(?whom)")
	if err != nil {
		log.Fatal(err)
	}
	rows, err := world.Match(pattern)
	if err != nil {
		log.Fatal(err)
	}
	for _, row := range rows {
		fmt.Println(store.Source(row["who"]), "knows", store.Source(row["whom"]))
	}

	// Output:
	// true
	// Alice knows Bob
}
