// Shared FuncMap→engine helper adaptation (GO-054). The generator hands every
// engine the same template.FuncMap it gives html/template; pongo2 and
// handlebars adapt what they can through reflection, and anything an engine
// cannot express is reported once per build instead of silently doing nothing
// (the old passthrough/ignore/recover behaviour).
package engine

import (
	"fmt"
	"html/template"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
)

var errType = reflect.TypeOf((*error)(nil)).Elem()

// helperAdapter wraps one FuncMap entry for dynamic invocation.
type helperAdapter struct {
	fn reflect.Value
}

// adaptHelper validates a FuncMap entry: it must be a func returning one
// value, optionally with a trailing error (the html/template contract).
func adaptHelper(fn interface{}) (*helperAdapter, error) {
	v := reflect.ValueOf(fn)
	if !v.IsValid() || v.Kind() != reflect.Func {
		return nil, fmt.Errorf("not a function")
	}
	t := v.Type()
	switch t.NumOut() {
	case 1:
	case 2:
		if !t.Out(1).Implements(errType) {
			return nil, fmt.Errorf("second return value must be error")
		}
	default:
		return nil, fmt.Errorf("must return one value (plus optional error)")
	}
	return &helperAdapter{fn: v}, nil
}

// accepts reports whether the helper can be called with n arguments.
func (a *helperAdapter) accepts(n int) bool {
	t := a.fn.Type()
	if t.IsVariadic() {
		return n >= t.NumIn()-1
	}
	return n == t.NumIn()
}

// call invokes the helper with best-effort argument conversion.
func (a *helperAdapter) call(args ...interface{}) (out interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	t := a.fn.Type()
	if !a.accepts(len(args)) {
		return nil, fmt.Errorf("want %d argument(s), got %d", t.NumIn(), len(args))
	}
	in := make([]reflect.Value, len(args))
	for i, arg := range args {
		var want reflect.Type
		if t.IsVariadic() && i >= t.NumIn()-1 {
			want = t.In(t.NumIn() - 1).Elem()
		} else {
			want = t.In(i)
		}
		cv, cerr := convertArg(arg, want)
		if cerr != nil {
			return nil, fmt.Errorf("argument %d: %w", i+1, cerr)
		}
		in[i] = cv
	}
	res := a.fn.Call(in)
	if len(res) == 2 && !res[1].IsNil() {
		return nil, res[1].Interface().(error)
	}
	return res[0].Interface(), nil
}

// convertArg coerces a template value into the parameter type. Numeric→string
// goes through fmt.Sprint (reflect's int→string conversion would produce a
// rune, not digits); everything else uses assignability/convertibility.
func convertArg(v interface{}, want reflect.Type) (reflect.Value, error) {
	if v == nil {
		return reflect.Zero(want), nil
	}
	rv := reflect.ValueOf(v)
	switch {
	case rv.Type().AssignableTo(want):
		return rv, nil
	case want.Kind() == reflect.String && rv.Kind() != reflect.String:
		return reflect.ValueOf(fmt.Sprint(v)).Convert(want), nil
	case rv.Type().ConvertibleTo(want):
		return rv.Convert(want), nil
	default:
		return reflect.Value{}, fmt.Errorf("cannot use %T as %s", v, want)
	}
}

// helperResultString renders a helper result for engines that expect strings;
// template.HTML keeps its pre-escaped meaning via the caller's safe wrapper.
func helperResultString(v interface{}) (s string, safe bool) {
	switch r := v.(type) {
	case template.HTML:
		return string(r), true
	case string:
		return r, false
	case nil:
		return "", false
	default:
		return fmt.Sprint(r), false
	}
}

// warnedHelpers deduplicates per-engine warnings so a build prints each
// message once, not once per template file.
var warnedHelpers sync.Map

// warnHelpersOnce prints one warning per engine+message pair per process.
func warnHelpersOnce(key, msg string) {
	if _, loaded := warnedHelpers.LoadOrStore(key+"|"+msg, true); !loaded {
		fmt.Fprintf(os.Stderr, "⚠️  %s\n", msg)
	}
}

// warnUnsupported reports helpers an engine cannot adapt (GO-054).
func warnUnsupported(engineName string, names []string) {
	if len(names) == 0 {
		return
	}
	sort.Strings(names)
	warnHelpersOnce(engineName, fmt.Sprintf("%s engine: %d template helper(s) unavailable: %s",
		engineName, len(names), strings.Join(names, ", ")))
}
