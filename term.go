package loom

// Term is an expression (§4). The interface is sealed.
//
// Parsing and resolution are separate phases, and the AST reflects that. Parse
// produces TermName for every identifier, because whether a name denotes an
// atom, a parameter or a definition is not a syntactic question. Resolve then
// replaces each TermName with what it actually denotes:
//
//	source → parse → Name/Lambda/Apply → collect definitions
//	       → resolve → Atom/Var/Def/Lambda/Apply → evaluate
type Term interface{ term() }

// TermAtom is a literal or a name that denotes nothing but itself.
type TermAtom struct{ Payload Payload }

// TermName is an unresolved identifier: what the parser produces before
// anything knows what it denotes. Evaluating one is an error, because it means
// the resolution phase was skipped.
type TermName struct{ Name string }

// TermVar is a reference to a value binding in the environment: a lambda
// parameter, or a name the base environment binds such as an intrinsic.
type TermVar struct{ Name string }

// TermDef is a reference to a top-level definition. Definitions are cells in
// the environment rather than values, which is what lets a definition refer to
// itself or to one written further down.
type TermDef struct{ Name string }

// TermLambda is an abstraction. Evaluating it captures the current environment
// in a closure (§6), and that environment travels with the closure: a function
// means what it meant where it was defined.
type TermLambda struct {
	Param string
	Body  Term
}

// TermApply is an application (§7). Application associates left, so f(x)(y)
// parses as TermApply{TermApply{f, x}, y}.
type TermApply struct{ Fn, Arg Term }

func (TermAtom) term()   {}
func (TermName) term()   {}
func (TermVar) term()    {}
func (TermDef) term()    {}
func (TermLambda) term() {}
func (TermApply) term()  {}
