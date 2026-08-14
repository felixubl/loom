package loom

// defState tracks whether a definition's term has been evaluated yet.
type defState uint8

const (
	defPending defState = iota
	defForcing
	defReady
)

// definition is a top-level binding's cell. The cell exists before its term is
// evaluated, which is what ties the knot: a definition can refer to itself, and
// two definitions can refer to each other, without any name being looked up in
// the caller's environment.
//
// A cell is forced the first time anything reads it, so a definition may also
// refer to one written further down.
type definition struct {
	name  string
	term  Term
	env   *Env
	value Value
	state defState
}

// Command is one statement of a Loom program.
//
// The read side and the write side are deliberately separate. Evaluate and
// Query read a world snapshot and change nothing; Assert, Retract and
// Transaction are the only things that change anything (§28, §33). Mutates
// reports which side a command is on.
type Command interface{ command() }

// Define binds a top-level name to the value of a term.
type Define struct {
	Name string
	Term Term
}

// Evaluate is a bare expression. It reads the world but never changes it.
type Evaluate struct{ Term Term }

// Query matches a pattern against the values held in a world snapshot (§21).
// It has its own statement syntax, but it is a read, not a mutation.
type Query struct{ Pattern Pattern }

// Assert evaluates a term and claims the resulting value (§29).
type Assert struct{ Term Term }

// Retract retracts one exact claim (§30).
type Retract struct{ Claim ClaimID }

// Transaction runs assertions and retractions as one atomic step (§31). Only
// Assert and Retract may appear inside one.
type Transaction struct{ Body []Command }

func (Define) command()      {}
func (Evaluate) command()    {}
func (Query) command()       {}
func (Assert) command()      {}
func (Retract) command()     {}
func (Transaction) command() {}

// Mutates reports whether a command changes the world. Only Assert, Retract and
// Transaction do. The boundary matters for caching and reactivity later: a read
// is repeatable against a snapshot, a write is not.
func Mutates(c Command) bool {
	switch c.(type) {
	case Assert, Retract, Transaction:
		return true
	}
	return false
}

// Program is a parsed sequence of statements.
type Program struct{ Commands []Command }

// Result is what running one command produced. Which fields are meaningful
// depends on the command: Value for Define and Evaluate, Claims for Assert and
// Transaction, Rows for Query.
type Result struct {
	Command Command
	Name    string
	Value   Value
	Claims  []ClaimID
	Rows    []Bindings
}

// Session is a Loom program in progress: a store plus an environment that grows
// as definitions are made.
//
// Redefining a name shadows it rather than overwriting it, so a closure made
// earlier keeps meaning what it meant. Mutual recursion needs both definitions
// in one program, because a statement cannot see names introduced after it.
//
// A Session belongs to one goroutine. The store beneath it may be shared.
type Session struct {
	store *Store
	env   *Env
}

func NewSession(s *Store) *Session {
	return &Session{store: s, env: s.Base()}
}

func (ss *Session) Store() *Store { return ss.store }

// Env returns the environment definitions have been made in.
func (ss *Session) Env() *Env { return ss.env }

// Eval resolves a term against the session's definitions and evaluates it.
func (ss *Session) Eval(t Term) (Value, error) {
	resolved, err := Resolve(t, ss.env)
	if err != nil {
		return nil, err
	}
	return ss.store.World().EvalIn(resolved, ss.env)
}

// Run executes one command, as a program of one statement.
func (ss *Session) Run(c Command) (Result, error) {
	results, err := ss.RunProgram(&Program{Commands: []Command{c}})
	if err != nil {
		return Result{}, err
	}
	return results[0], nil
}

// RunProgram executes a program in three phases: collect every top-level name,
// resolve every term against them, then run the statements in source order.
//
// Collecting first is what lets a definition refer to one written further down,
// and lets two definitions refer to each other. Running in source order is what
// keeps a definition's world reads consistent with the writes around it.
func (ss *Session) RunProgram(p *Program) ([]Result, error) {
	// Phase 1: collect. Every top-level cell exists before any body is resolved.
	env := ss.env
	cells := map[string]*definition{}
	for _, c := range p.Commands {
		d, ok := c.(Define)
		if !ok {
			continue
		}
		if _, duplicate := cells[d.Name]; duplicate {
			return nil, errorf(DuplicateDefinition, "%s is defined twice in one program", d.Name)
		}
		cell := &definition{name: d.Name}
		cells[d.Name] = cell
		env = env.bindDef(cell)
	}
	for _, cell := range cells {
		cell.env = env
	}

	// Phase 2: resolve, with every top-level name already in scope.
	resolved := make([]Command, len(p.Commands))
	for i, c := range p.Commands {
		rc, err := resolveCommand(c, env)
		if err != nil {
			return nil, err
		}
		resolved[i] = rc
		if d, ok := rc.(Define); ok {
			cells[d.Name].term = d.Term
		}
	}

	// Phase 3: run.
	ss.env = env
	out := make([]Result, 0, len(resolved))
	for _, c := range resolved {
		r, err := ss.run(c, cells)
		if err != nil {
			return out, err
		}
		out = append(out, r)
	}
	return out, nil
}

func resolveCommand(c Command, env *Env) (Command, error) {
	switch x := c.(type) {
	case Define:
		t, err := Resolve(x.Term, env)
		if err != nil {
			return nil, err
		}
		return Define{Name: x.Name, Term: t}, nil
	case Evaluate:
		t, err := Resolve(x.Term, env)
		if err != nil {
			return nil, err
		}
		return Evaluate{Term: t}, nil
	case Assert:
		t, err := Resolve(x.Term, env)
		if err != nil {
			return nil, err
		}
		return Assert{Term: t}, nil
	case Transaction:
		body := make([]Command, len(x.Body))
		for i, inner := range x.Body {
			r, err := resolveCommand(inner, env)
			if err != nil {
				return nil, err
			}
			body[i] = r
		}
		return Transaction{Body: body}, nil
	case Retract, Query:
		return c, nil
	}
	return nil, errorf(PrimitiveError, "not a command: %T", c)
}

func (ss *Session) run(c Command, cells map[string]*definition) (Result, error) {
	switch x := c.(type) {
	case Define:
		cell, ok := cells[x.Name]
		if !ok {
			return Result{}, errorf(UnboundVariable, "definition %s was not collected", x.Name)
		}
		v, err := ss.store.World().evaluator().force(cell, 0)
		if err != nil {
			return Result{}, err
		}
		return Result{Command: c, Name: x.Name, Value: v}, nil

	case Evaluate:
		v, err := ss.store.World().EvalIn(x.Term, ss.env)
		if err != nil {
			return Result{}, err
		}
		return Result{Command: c, Value: v}, nil

	case Query:
		rows, err := ss.store.World().Match(x.Pattern)
		if err != nil {
			return Result{}, err
		}
		return Result{Command: c, Rows: rows}, nil

	case Assert:
		return ss.atomically(c, []Command{x})

	case Retract:
		return ss.atomically(c, []Command{x})

	case Transaction:
		return ss.atomically(c, x.Body)
	}
	return Result{}, errorf(PrimitiveError, "not a command: %T", c)
}

// atomically runs a body of writes as one transaction. Nothing is published
// unless every command in it succeeds (§31).
func (ss *Session) atomically(c Command, body []Command) (Result, error) {
	tx := ss.store.Begin()
	var claims []ClaimID
	for _, inner := range body {
		switch y := inner.(type) {
		case Assert:
			// Evaluated against the staged world, so a later assertion sees an
			// earlier one (§32).
			v, err := tx.World().EvalIn(y.Term, ss.env)
			if err != nil {
				tx.Discard()
				return Result{}, err
			}
			id, err := Persist(v)
			if err != nil {
				tx.Discard()
				return Result{}, err
			}
			claim, err := tx.AssertValue(id, nil)
			if err != nil {
				tx.Discard()
				return Result{}, err
			}
			claims = append(claims, claim)
		case Retract:
			if err := tx.Retract(y.Claim); err != nil {
				tx.Discard()
				return Result{}, err
			}
		default:
			tx.Discard()
			return Result{}, errorf(PrimitiveError, "only assert and retract may appear in a transaction, found %T", inner)
		}
	}
	if err := tx.Commit(); err != nil {
		return Result{}, err
	}
	return Result{Command: c, Claims: claims}, nil
}
