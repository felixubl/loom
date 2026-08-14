package loom

// Term is a parsed expression (§4). The interface is sealed: atoms, variable
// references, abstractions and applications are the only terms there are.
type Term interface{ term() }

// TermAtom is a literal or named atom.
type TermAtom struct{ Payload Payload }

// TermVar references a lambda parameter. Parse only emits one for an
// identifier that some enclosing lambda binds, so a free identifier is an atom
// and unbound_variable can only come from a hand-built term.
type TermVar struct{ Name string }

// TermLambda is an abstraction. Evaluating it captures the current environment
// in a closure (§6).
type TermLambda struct {
	Param string
	Body  Term
}

// TermApply is an application (§7). Application associates left, so f(x)(y)
// parses as TermApply{TermApply{f, x}, y}.
type TermApply struct{ Fn, Arg Term }

func (TermAtom) term()   {}
func (TermVar) term()    {}
func (TermLambda) term() {}
func (TermApply) term()  {}
