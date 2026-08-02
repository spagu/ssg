package ai

import (
	"fmt"
	"strconv"
	"strings"
)

// Eval evaluates an `ifs` guard against a page's fields. Empty ⇒ true (always
// run). The grammar is deliberately small: OR-separated groups of AND-separated
// comparisons, each `field OP value`, evaluated left to right (no parentheses).
//
//	lang == en AND status == publish
//	category == news OR category == blog
//
// OP is one of ==, !=, contains, >, <, >=, <=. The left side is a field name
// looked up in vars; the right side is a literal (surrounding quotes optional).
func Eval(ifs string, vars map[string]string) (bool, error) {
	ifs = strings.TrimSpace(ifs)
	if ifs == "" {
		return true, nil
	}
	for _, orGroup := range splitOp(ifs, " OR ") {
		all := true
		for _, cmp := range splitOp(orGroup, " AND ") {
			ok, err := evalComparison(cmp, vars)
			if err != nil {
				return false, err
			}
			if !ok {
				all = false
				break
			}
		}
		if all {
			return true, nil
		}
	}
	return false, nil
}

// splitOp splits s on a case-insensitive whole-word operator (" AND "/" OR ").
func splitOp(s, op string) []string {
	var out []string
	rest := s
	for {
		i := indexFold(rest, op)
		if i < 0 {
			out = append(out, strings.TrimSpace(rest))
			return out
		}
		out = append(out, strings.TrimSpace(rest[:i]))
		rest = rest[i+len(op):]
	}
}

func indexFold(s, sub string) int {
	return strings.Index(strings.ToUpper(s), strings.ToUpper(sub))
}

var operators = []string{">=", "<=", "==", "!=", ">", "<", " contains "}

func evalComparison(expr string, vars map[string]string) (bool, error) {
	for _, op := range operators {
		needle := op
		if op == " contains " {
			needle = " contains "
		}
		idx := indexFold(expr, needle)
		if idx < 0 {
			continue
		}
		field := strings.TrimSpace(expr[:idx])
		want := unquote(strings.TrimSpace(expr[idx+len(needle):]))
		got := vars[field]
		return compare(strings.TrimSpace(op), got, want), nil
	}
	return false, fmt.Errorf("ai ifs: cannot parse condition %q (want `field OP value`)", expr)
}

func compare(op, got, want string) bool {
	switch op {
	case "==":
		return got == want
	case "!=":
		return got != want
	case "contains":
		return strings.Contains(got, want)
	case ">", "<", ">=", "<=":
		return compareOrdered(op, got, want)
	}
	return false
}

// compareOrdered compares numerically when both sides parse as numbers, else
// lexicographically.
func compareOrdered(op, got, want string) bool {
	gf, gErr := strconv.ParseFloat(got, 64)
	wf, wErr := strconv.ParseFloat(want, 64)
	if gErr == nil && wErr == nil {
		switch op {
		case ">":
			return gf > wf
		case "<":
			return gf < wf
		case ">=":
			return gf >= wf
		case "<=":
			return gf <= wf
		}
	}
	switch op {
	case ">":
		return got > want
	case "<":
		return got < want
	case ">=":
		return got >= want
	case "<=":
		return got <= want
	}
	return false
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}
