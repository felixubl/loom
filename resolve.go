package loom

// Resolve is the phase between parsing and evaluation. It replaces each parsed
// name with the thing that name denotes, in this precedence:
//
//  1. a lexically bound lambda parameter
//  2. a top-level definition in scope
//  3. a base-environment binding, such as an intrinsic
//  4. otherwise the atom of that name
//
// The fallback in step 4 is what keeps knows(Alice)(Bob) graph structure with
// nobody having declared knows, while identity(Alice) still calls a function.
//
// Deciding this here rather than at evaluation time is what makes scoping
// lexical. A name is bound to what was in scope where it was written, so a
// closure cannot have its meaning changed by whatever the caller has in scope.
// It is also what allows a definition to refer to one written further down: the
// caller creates every top-level cell in env before resolving any body.
func Resolve(t Term, env *Env) (Term, error) {
	r := &resolver{env: env}
	return r.resolve(t, 0)
}

type resolver struct {
	env    *Env
	params []string
}

func (r *resolver) bound(name string) bool {
	for i := len(r.params) - 1; i >= 0; i-- {
		if r.params[i] == name {
			return true
		}
	}
	return false
}

func (r *resolver) resolve(t Term, depth int) (Term, error) {
	if depth > maxNesting {
		return nil, errorf(ResourceLimit, "term nested more than %d deep", maxNesting)
	}
	switch x := t.(type) {
	case TermName:
		if r.bound(x.Name) {
			return TermVar{Name: x.Name}, nil
		}
		if b := r.env.find(x.Name); b != nil {
			if b.def != nil {
				return TermDef{Name: x.Name}, nil
			}
			return TermVar{Name: x.Name}, nil
		}
		return TermAtom{Payload: NameAtom(x.Name)}, nil

	case TermLambda:
		r.params = append(r.params, x.Param)
		body, err := r.resolve(x.Body, depth+1)
		r.params = r.params[:len(r.params)-1]
		if err != nil {
			return nil, err
		}
		return TermLambda{Param: x.Param, Body: body}, nil

	case TermApply:
		fn, err := r.resolve(x.Fn, depth+1)
		if err != nil {
			return nil, err
		}
		arg, err := r.resolve(x.Arg, depth+1)
		if err != nil {
			return nil, err
		}
		return TermApply{Fn: fn, Arg: arg}, nil

	case TermAtom, TermVar, TermDef:
		return t, nil

	case nil:
		return nil, errorf(UnresolvedName, "missing term")
	}
	return nil, errorf(UnresolvedName, "not a term: %T", t)
}
