package gointerop

import (
	"fmt"
	"go/types"
)

// ResultKind describes the return shape of a Go function supported by RIP BASIC.
type ResultKind int

const (
	ResultVoid     ResultKind = iota // func(...)
	ResultError                      // func(...) error
	ResultValue                      // func(...) T
	ResultValueErr                   // func(...) (T, error)
)

// ParamInfo describes one Go function parameter.
type ParamInfo struct {
	Name      string
	Type      types.Type // for variadic params, this is the slice type []E
	IsContext bool       // true for context.Context (auto-injected c.Context())
	Variadic  bool       // true for the final ...E parameter
	ElemType  types.Type // for variadic params, E
}

// FuncSig is the validated interop view of a Go function signature.
type FuncSig struct {
	Name       string
	Params     []ParamInfo
	MinArgs    int // minimum BASIC arguments (excluding context.Context)
	MaxArgs    int // maximum BASIC arguments (-1 if variadic)
	Variadic   bool
	ResultKind ResultKind
	ResultType types.Type // non-nil for ResultValue and ResultValueErr
}

var errorIfaceType = types.Universe.Lookup("error").Type()

// IsContextType reports whether t is context.Context.
func IsContextType(t types.Type) bool {
	u := types.Unalias(t)
	named, ok := u.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == "context" && obj.Name() == "Context"
}

// IsErrorType reports whether t is the built-in Go error interface.
func IsErrorType(t types.Type) bool {
	return types.Identical(types.Unalias(t), errorIfaceType)
}

// IsByteSlice reports whether t is an unnamed []byte / []uint8 slice.
func IsByteSlice(t types.Type) bool {
	sl, ok := types.Unalias(t).(*types.Slice)
	if !ok {
		return false
	}
	b, ok := types.Unalias(sl.Elem()).(*types.Basic)
	return ok && b.Kind() == types.Uint8
}

// AnalyzeFunc validates a package-level Go function and returns its interop signature.
func AnalyzeFunc(fn *types.Func) (*FuncSig, error) {
	if fn == nil {
		return nil, fmt.Errorf("missing Go function")
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return nil, fmt.Errorf("Go symbol %s is not a function", fn.Name())
	}
	return AnalyzeSig(fn.Name(), sig)
}

// AnalyzeSig validates a Go function signature for RIP BASIC interop
// (Requirements 5.5–5.6, design §4.10).
func AnalyzeSig(name string, sig *types.Signature) (*FuncSig, error) {
	if sig == nil {
		return nil, fmt.Errorf("Go function %s has no signature", name)
	}
	if sig.TypeParams().Len() > 0 {
		return nil, fmt.Errorf("Go function %s is generic (has type parameters), which IMPORT … FROM GO does not support — write a non-generic Go wrapper function if you need it", name)
	}

	fs := &FuncSig{
		Name:     name,
		Variadic: sig.Variadic(),
	}

	params := sig.Params()
	nParams := params.Len()
	for i := 0; i < nParams; i++ {
		p := params.At(i)
		pt := p.Type()
		isVar := sig.Variadic() && i == nParams-1
		isCtx := !isVar && IsContextType(pt)

		pi := ParamInfo{
			Name:      p.Name(),
			Type:      pt,
			IsContext: isCtx,
			Variadic:  isVar,
		}
		if isVar {
			sl, ok := types.Unalias(pt).(*types.Slice)
			if !ok {
				return nil, fmt.Errorf("Go function %s has unsupported variadic parameter type %s", name, pt)
			}
			pi.ElemType = sl.Elem()
			if err := validateParamType(name, sl.Elem()); err != nil {
				return nil, err
			}
		} else if !isCtx {
			if err := validateParamType(name, pt); err != nil {
				return nil, err
			}
			fs.MinArgs++
		}
		fs.Params = append(fs.Params, pi)
	}
	if fs.Variadic {
		fs.MaxArgs = -1
	} else {
		fs.MaxArgs = fs.MinArgs
	}

	results := sig.Results()
	switch results.Len() {
	case 0:
		fs.ResultKind = ResultVoid
	case 1:
		rt := results.At(0).Type()
		if IsErrorType(rt) {
			fs.ResultKind = ResultError
		} else {
			if err := validateResultType(name, rt); err != nil {
				return nil, err
			}
			fs.ResultKind = ResultValue
			fs.ResultType = rt
		}
	case 2:
		r0 := results.At(0).Type()
		r1 := results.At(1).Type()
		if !IsErrorType(r1) {
			return nil, fmt.Errorf("Go function %s returns 2 non-error values (%s, %s); RIP BASIC supports at most one value plus an optional trailing error", name, r0, r1)
		}
		if err := validateResultType(name, r0); err != nil {
			return nil, err
		}
		fs.ResultKind = ResultValueErr
		fs.ResultType = r0
	default:
		return nil, fmt.Errorf("Go function %s returns %d values; RIP BASIC supports at most one value plus an optional trailing error", name, results.Len())
	}

	return fs, nil
}

// ValidateNewType checks that tn can be constructed with NEW(pkg.Type).
func ValidateNewType(pkgLabel string, tn *types.TypeName) error {
	if tn == nil {
		return fmt.Errorf("missing Go type for NEW()")
	}
	if !tn.Exported() {
		return fmt.Errorf("Go type %s.%s is not exported", pkgLabel, tn.Name())
	}
	t := types.Unalias(tn.Type())
	if named, ok := t.(*types.Named); ok {
		if named.TypeParams().Len() > 0 && named.TypeArgs().Len() == 0 {
			return fmt.Errorf("Go type %s.%s is generic (has type parameters) and cannot be constructed with NEW()", pkgLabel, tn.Name())
		}
	}
	switch t.Underlying().(type) {
	case *types.Interface:
		return fmt.Errorf("Go type %s.%s is an interface; NEW() only constructs concrete Go types like structs", pkgLabel, tn.Name())
	case *types.Chan:
		return fmt.Errorf("Go type %s.%s is a channel, which IMPORT … FROM GO does not support", pkgLabel, tn.Name())
	case *types.Signature:
		return fmt.Errorf("Go type %s.%s is a function type, which NEW() cannot construct", pkgLabel, tn.Name())
	}
	return nil
}

func validateParamType(fnName string, t types.Type) error {
	return validateType(fnName, t, true, map[types.Type]bool{})
}

func validateResultType(fnName string, t types.Type) error {
	return validateType(fnName, t, false, map[types.Type]bool{})
}

func validateType(fnName string, t types.Type, isParam bool, seen map[types.Type]bool) error {
	if t == nil || seen[t] {
		return nil
	}
	seen[t] = true

	role := "return"
	if isParam {
		role = "parameter"
	}

	switch x := t.(type) {
	case *types.Alias:
		if obj := x.Obj(); obj != nil && obj.Pkg() != nil && !obj.Exported() && isParam {
			return validateType(fnName, types.Unalias(x), isParam, seen)
		}
		if obj := x.Obj(); obj != nil && obj.Pkg() == nil && obj.Name() == "any" {
			return nil
		}
		return validateType(fnName, types.Unalias(x), isParam, seen)

	case *types.TypeParam:
		return fmt.Errorf("Go function %s uses generic type parameter %s, which IMPORT … FROM GO does not support", fnName, x.Obj().Name())

	case *types.Chan:
		return fmt.Errorf("Go function %s has a channel %s (%s), which IMPORT … FROM GO does not support — wrap it in a synchronous Go helper", fnName, role, t)

	case *types.Signature:
		return fmt.Errorf("Go function %s has a function/callback %s (%s), which IMPORT … FROM GO does not support — wrap it in a Go helper", fnName, role, t)

	case *types.Basic:
		switch x.Kind() {
		case types.UnsafePointer:
			return fmt.Errorf("Go function %s uses unsafe.Pointer as a %s, which IMPORT … FROM GO does not support", fnName, role)
		case types.Complex64, types.Complex128:
			return fmt.Errorf("Go function %s uses complex number type %s as a %s, which IMPORT … FROM GO does not support", fnName, x.Name(), role)
		}
		return nil

	case *types.Pointer:
		return validateType(fnName, x.Elem(), isParam, seen)

	case *types.Slice:
		return validateType(fnName, x.Elem(), isParam, seen)

	case *types.Array:
		return validateType(fnName, x.Elem(), isParam, seen)

	case *types.Map:
		if err := validateType(fnName, x.Key(), isParam, seen); err != nil {
			return err
		}
		return validateType(fnName, x.Elem(), isParam, seen)

	case *types.Named:
		if IsContextType(x) || IsErrorType(x) {
			return nil
		}
		if isParam && x.Obj() != nil && x.Obj().Pkg() != nil && !x.Obj().Exported() {
			return fmt.Errorf("Go function %s takes unexported type %s, which cannot be referenced from generated code", fnName, x)
		}
		if x.TypeParams().Len() > 0 && x.TypeArgs().Len() == 0 {
			return fmt.Errorf("Go function %s uses generic type %s, which IMPORT … FROM GO does not support", fnName, x)
		}
		// Reject named channel or function types, while allowing structs/interfaces
		// even if they happen to contain internal channel or callback fields
		// (like *http.Request or *http.Client).
		switch x.Underlying().(type) {
		case *types.Chan:
			return fmt.Errorf("Go function %s has a channel %s (%s), which IMPORT … FROM GO does not support", fnName, role, x)
		case *types.Signature:
			return fmt.Errorf("Go function %s has a function/callback %s (%s), which IMPORT … FROM GO does not support", fnName, role, x)
		}
		return nil

	case *types.Struct:
		// Anonymous structs are valid as Host values or struct conversions.
		return nil

	case *types.Interface:
		return nil
	}
	return nil
}
