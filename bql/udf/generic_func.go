package udf

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"

	"gopkg.in/sensorbee/sensorbee.v0/core"
	"gopkg.in/sensorbee/sensorbee.v0/data"
)

// ConvertGeneric creates a new UDF from various form of functions. Arguments
// of the function don't have to be tuple types, but some standard types are
// allowed. The UDF returned provide a weak type conversion, that is it uses
// data.To{Type} function to convert values. Therefore, a string may be
// passed as an integer or vice versa. If the function wants to provide
// strict type conversion, generate UDF by Func function.
//
// Acceptable types:
//	- bool
//	- standard integers
//	- standard floats
//	- string
//	- time.Time
//	- data.Bool, data.Int, data.Float, data.String, data.Blob,
//	  data.Timestamp, data.Array, data.Map, data.Value
//	- a slice of types above
func ConvertGeneric(function interface{}) (UDF, error) {
	t := reflect.TypeOf(function)
	if t.Kind() != reflect.Func {
		return nil, errors.New("the argument must be a function")
	}

	numArgs := t.NumIn()
	if genericFuncHasContext(t) {
		numArgs--
	}

	return convertGenericAggregate(function, make([]bool, numArgs), false)
}

// MustConvertGeneric is like ConvertGeneric, but panics on errors.
func MustConvertGeneric(function interface{}) UDF {
	f, err := ConvertGeneric(function)
	if err != nil {
		panic(err)
	}
	return f
}

// ConvertGenericAggregate creates a new aggregate UDF from various form of
// functions. aggParams argument is used to indicate which arguments of the
// function are aggregation parameter.
// receives aggregation parameter.
// Supported and acceptable types are the same as ConvertGeneric.
func ConvertGenericAggregate(function interface{}, aggParams []bool) (UDF, error) {
	return convertGenericAggregate(function, aggParams, true)
}

func convertGenericAggregate(function interface{}, aggParams []bool, isAggregate bool) (UDF, error) {
	t := reflect.TypeOf(function)
	if t.Kind() != reflect.Func {
		return nil, errors.New("the argument must be a function")
	}

	copiedParams := make([]bool, len(aggParams))
	copy(copiedParams, aggParams)
	g := &genericFunc{
		function:             reflect.ValueOf(function),
		hasContext:           genericFuncHasContext(t),
		variadic:             t.IsVariadic(),
		arity:                t.NumIn(),
		aggregationParameter: copiedParams,
	}

	if g.hasContext {
		g.arity--
	}

	if isAggregate {
		if g.arity == 0 {
			return nil, errors.New("UDAF must have at least one argument")
		}

		hasTrue := false
		for _, b := range aggParams {
			if b {
				hasTrue = true
				break
			}
		}
		if !hasTrue {
			return nil, errors.New("the function doesn't have an aggregation parameter")
		}
	}

	if g.arity != len(aggParams) {
		return nil, errors.New("the aggParams must have the same number of arguments of the function")
	}

	for i := 0; i < g.arity; i++ {
		if !aggParams[i] {
			continue
		}

		in := i
		if g.hasContext {
			in++
		}
		if t.In(in).Kind() != reflect.Slice {
			return nil, fmt.Errorf("the %v-th parameter for aggregation must be slice", i+1)
		}
	}

	hasError, err := checkGenericFuncReturnTypes(t)
	if err != nil {
		return nil, err
	}
	g.hasError = hasError

	convs, err := createGenericConverters(t, t.NumIn()-g.arity)
	if err != nil {
		return nil, err
	}
	g.converters = convs

	return g, nil
}

// MustConvertGenericAggregate is like ConvertGenericAggregate,
// but panics on errors.
func MustConvertGenericAggregate(function interface{}, aggParams []bool) UDF {
	f, err := ConvertGenericAggregate(function, aggParams)
	if err != nil {
		panic(err)
	}
	return f
}

func checkGenericFuncReturnTypes(t reflect.Type) (bool, error) {
	hasError := false

	switch n := t.NumOut(); n {
	case 2:
		if !t.Out(1).Implements(reflect.TypeOf(func(error) {}).In(0)) {
			return false, fmt.Errorf("the second return value must be an error: %v", t.Out(1))
		}
		hasError = true
		fallthrough

	case 1:
		out := t.Out(0)
		if out.Kind() == reflect.Interface {
			// data.Value is the only interface which is accepted.
			if !out.Implements(reflect.TypeOf(data.NewValue).Out(0)) {
				return false, fmt.Errorf("the return value isn't convertible to data.Value")
			}
		}
		if _, err := data.NewValue(reflect.Zero(out).Interface()); err != nil {
			return false, fmt.Errorf("the return value isn't convertible to data.Value")
		}

	default:
		return false, fmt.Errorf("the number of return values must be 1 or 2: %v", n)
	}
	return hasError, nil
}

func genericFuncHasContext(t reflect.Type) bool {
	if t.NumIn() == 0 {
		return false
	}
	c := t.In(0)
	return reflect.TypeOf(&core.Context{}).AssignableTo(c)
}

func createGenericConverters(t reflect.Type, argStart int) ([]argumentConverter, error) {
	variadic := t.IsVariadic()
	convs := make([]argumentConverter, 0, t.NumIn()-argStart)
	for i := argStart; i < t.NumIn(); i++ {
		arg := t.In(i)
		if i == t.NumIn()-1 && variadic {
			arg = arg.Elem()
		}

		c, err := genericFuncArgumentConverter(arg)
		if err != nil {
			return nil, err
		}
		convs = append(convs, c)
	}
	return convs, nil
}

type argumentConverter func(data.Value) (interface{}, error)

func genericFuncArgumentConverter(t reflect.Type) (argumentConverter, error) {
	// TODO: this function is too long.
	switch t.Kind() {
	case reflect.Bool:
		return func(v data.Value) (interface{}, error) {
			return data.ToBool(v)
		}, nil

	case reflect.Int:
		return func(v data.Value) (interface{}, error) {
			i, err := data.ToInt(v)
			if err != nil {
				return nil, err
			}

			if i < -1^int64(^uint(0)>>1) {
				return nil, fmt.Errorf("%v is too small for int", i)
			} else if i > int64(^uint(0)>>1) {
				return nil, fmt.Errorf("%v is too big for int", i)
			}
			return int(i), nil
		}, nil

	case reflect.Int8:
		return func(v data.Value) (interface{}, error) {
			i, err := data.ToInt(v)
			if err != nil {
				return nil, err
			}

			if i < math.MinInt8 {
				return nil, fmt.Errorf("%v is too small for int8", i)
			} else if i > math.MaxInt8 {
				return nil, fmt.Errorf("%v is too big for int8", i)
			}
			return int8(i), nil
		}, nil

	case reflect.Int16:
		return func(v data.Value) (interface{}, error) {
			i, err := data.ToInt(v)
			if err != nil {
				return nil, err
			}

			if i < math.MinInt16 {
				return nil, fmt.Errorf("%v is too small for int16", i)
			} else if i > math.MaxInt16 {
				return nil, fmt.Errorf("%v is too big for int16", i)
			}
			return int16(i), nil
		}, nil

	case reflect.Int32:
		return func(v data.Value) (interface{}, error) {
			i, err := data.ToInt(v)
			if err != nil {
				return nil, err
			}

			if i < math.MinInt32 {
				return nil, fmt.Errorf("%v is too small for int32", i)
			} else if i > math.MaxInt32 {
				return nil, fmt.Errorf("%v is too big for int32", i)
			}
			return int32(i), nil
		}, nil

	case reflect.Int64:
		return func(v data.Value) (interface{}, error) {
			return data.ToInt(v)
		}, nil

	case reflect.Uint:
		return func(v data.Value) (interface{}, error) {
			i, err := data.ToInt(v)
			if err != nil {
				return nil, err
			}

			if i < 0 {
				return nil, fmt.Errorf("%v is too small for uint", i)
			} else if i > int64(^uint(0)>>1) {
				return nil, fmt.Errorf("%v is too big for uint", i)
			}
			return uint(i), nil
		}, nil

	case reflect.Uint8:
		return func(v data.Value) (interface{}, error) {
			i, err := data.ToInt(v)
			if err != nil {
				return nil, err
			}

			if i < 0 {
				return nil, fmt.Errorf("%v is too small for uint8", i)
			} else if i > math.MaxUint8 {
				return nil, fmt.Errorf("%v is too big for uint8", i)
			}
			return uint8(i), nil
		}, nil

	case reflect.Uint16:
		return func(v data.Value) (interface{}, error) {
			i, err := data.ToInt(v)
			if err != nil {
				return nil, err
			}

			if i < 0 {
				return nil, fmt.Errorf("%v is too small for uint16", i)
			} else if i > math.MaxUint16 {
				return nil, fmt.Errorf("%v is too big for uint16", i)
			}
			return uint16(i), nil
		}, nil

	case reflect.Uint32:
		return func(v data.Value) (interface{}, error) {
			i, err := data.ToInt(v)
			if err != nil {
				return nil, err
			}

			if i < 0 {
				return nil, fmt.Errorf("%v is too small for uint32", i)
			} else if i > math.MaxUint32 {
				return nil, fmt.Errorf("%v is too big for uint32", i)
			}
			return uint32(i), nil
		}, nil

	case reflect.Uint64:
		return func(v data.Value) (interface{}, error) {
			i, err := data.ToInt(v)
			if err != nil {
				return nil, err
			}

			if i < 0 {
				return nil, fmt.Errorf("%v is too small for uint64", i)
			}
			return uint64(i), nil
		}, nil

	case reflect.Float32:
		return func(v data.Value) (interface{}, error) {
			f, err := data.ToFloat(v)
			if err != nil {
				return nil, err
			}
			return float32(f), err
		}, nil

	case reflect.Float64:
		return func(v data.Value) (interface{}, error) {
			return data.ToFloat(v)
		}, nil

	case reflect.String:
		return func(v data.Value) (interface{}, error) {
			return data.ToString(v)
		}, nil

	case reflect.Slice:
		elemType := t.Elem()
		if elemType.Kind() == reflect.Uint8 {
			// process this as a blob
			return func(v data.Value) (interface{}, error) {
				// This function explicitly returns nil to avoid returning
				// nils having non-empty type information for later nil
				// equality checks.
				res, err := data.ToBlob(v)
				if err != nil {
					return nil, err
				}
				if res == nil {
					return nil, err
				}
				return res, nil
			}, nil
		}

		c, err := genericFuncArgumentConverter(elemType)
		if err != nil {
			return nil, err
		}
		return func(v data.Value) (interface{}, error) {
			a, err := data.AsArray(v)
			if err != nil {
				return nil, err
			}
			res := reflect.MakeSlice(t, 0, len(a))
			for _, elem := range a {
				e, err := c(elem)
				if err != nil {
					return nil, err
				}
				res = reflect.Append(res, reflect.ValueOf(e))
			}
			return res.Interface(), nil // res will never be nil.
		}, nil

	default:
		switch reflect.Zero(t).Interface().(type) {
		case data.Map:
			return func(v data.Value) (interface{}, error) {
				res, err := data.AsMap(v)
				if err != nil {
					return nil, err
				}
				if res == nil {
					return nil, err
				}
				return res, nil
			}, nil

		case time.Time:
			return func(v data.Value) (interface{}, error) {
				return data.ToTimestamp(v)
			}, nil

		default:
			if t.Implements(reflect.TypeOf(data.NewValue).Out(0)) { // data.Value
				// Zero(interface) returns nil and type assertion doesn't work for it.
				return func(v data.Value) (interface{}, error) {
					if v == nil {
						return nil, nil // Erase type information (data.Value) from nil
					}
					return v, nil
				}, nil
			}
			// other tuple types are covered in Kind() switch above
			return nil, fmt.Errorf("unsupported type: %v", t)
		}
	}
}

type genericFunc struct {
	function reflect.Value

	hasContext bool
	hasError   bool
	variadic   bool

	// arity is the number of arguments. If the function is variadic, arity
	// counts the last variadic parameter. For example, if the function is
	// func(int, float, ...string), arity is 3. It doesn't count Context.
	arity int

	// aggregationParameter have the same length as the number of arguments
	// excluding the *core.Context.
	// The values are returned by IsAggregationParameter method.
	// If the aggregationParameter[n] boolean value is true, the n-th function
	// argument receives aggregation parameter.
	aggregationParameter []bool

	converters []argumentConverter
}

func (g *genericFunc) Call(ctx *core.Context, args ...data.Value) (data.Value, error) {
	out, err := g.call(ctx, args...)
	if err != nil {
		return nil, err
	}

	if g.hasError {
		if !out[1].IsNil() {
			return nil, out[1].Interface().(error)
		}
	}
	return data.NewValue(out[0].Interface())
}

func (g *genericFunc) call(ctx *core.Context, args ...data.Value) ([]reflect.Value, error) {
	if len(args) < g.arity {
		if g.variadic && len(args) == g.arity-1 {
			// having no variadic parameter is ok.
		} else {
			return nil, fmt.Errorf("insufficient number of argumetns")
		}

	} else if len(args) != g.arity && !g.variadic {
		return nil, fmt.Errorf("too many arguments")
	}

	in := make([]reflect.Value, 0, len(args)+1) // +1 for context
	if g.hasContext {
		in = append(in, reflect.ValueOf(ctx))
	}

	variadicBegin := g.arity
	if g.variadic {
		variadicBegin--
	}

	for i := 0; i < variadicBegin; i++ {
		v, err := g.converters[i](args[i])
		if err != nil {
			return nil, err
		}
		in = append(in, reflect.ValueOf(v))
	}
	for i := variadicBegin; i < len(args); i++ {
		v, err := g.converters[len(g.converters)-1](args[i])
		if err != nil {
			return nil, err
		}
		in = append(in, reflect.ValueOf(v))
	}
	return g.function.Call(in), nil
}

func (g *genericFunc) Accept(arity int) bool {
	if arity < g.arity {
		if g.variadic && arity == g.arity-1 {
			// having no variadic parameter is ok.
		} else {
			return false
		}

	} else if arity != g.arity && !g.variadic {
		return false
	}
	return true
}

func (g *genericFunc) IsAggregationParameter(k int) bool {
	if len(g.aggregationParameter) <= k {
		if g.variadic {
			return g.aggregationParameter[len(g.aggregationParameter)-1]
		}
		return false
	}
	return g.aggregationParameter[k]
}
