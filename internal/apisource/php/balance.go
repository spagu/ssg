package php

import "strconv"

// closers maps each opening bracket to the one that closes it.
var closers = map[string]string{"{": "}", "(": ")", "[": "]"}

// checkBalance reports the first bracket that is closed by the wrong one,
// closes nothing or is never closed. The declaration pass relies on
// brackets pairing up to skip bodies.
func checkBalance(toks []token) error {
	var open []token
	for _, t := range toks {
		if t.kind != tokPunct {
			continue
		}
		switch t.text {
		case "{", "(", "[":
			open = append(open, t)
		case "}", ")", "]":
			if len(open) == 0 {
				return &lexError{line: t.line, msg: "unbalanced " + t.text + ": nothing to close"}
			}
			top := open[len(open)-1]
			if closers[top.text] != t.text {
				return &lexError{line: t.line, msg: "unbalanced " + t.text + ": " + top.text + " opened on line " + strconv.Itoa(top.line)}
			}
			open = open[:len(open)-1]
		}
	}
	if len(open) > 0 {
		t := open[len(open)-1]
		return &lexError{line: t.line, msg: "unbalanced " + t.text + ": never closed"}
	}
	return nil
}
