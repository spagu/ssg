package snapenv

import (
	"errors"
	"testing"
)

func TestRootOf(t *testing.T) {
	t.Setenv("SNAP", "/snap/ssg/42")
	tests := []struct {
		exe, want string
	}{
		{"/snap/ssg/42/bin/ssg", "/snap/ssg/42"},
		{"/snap/ssg/421/bin/ssg", ""}, // a sibling revision is not under the root
		{"/snap/go/9/bin/go", ""},     // another snap's $SNAP is not ours
		{"/usr/bin/ssg", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := RootOf(tt.exe); got != tt.want {
			t.Errorf("RootOf(%q) = %q, want %q", tt.exe, got, tt.want)
		}
	}
	t.Setenv("SNAP", "")
	if got := RootOf("/snap/ssg/42/bin/ssg"); got != "" {
		t.Errorf("without $SNAP RootOf = %q, want empty", got)
	}
}

func TestRoot(t *testing.T) {
	t.Setenv("SNAP", "/snap/ssg/42")
	defer func(orig func() (string, error)) { executable = orig }(executable)

	executable = func() (string, error) { return "/snap/ssg/42/bin/ssg", nil }
	if got := Root(); got != "/snap/ssg/42" {
		t.Errorf("Root() = %q, want the snap root", got)
	}
	executable = func() (string, error) { return "", errors.New("no /proc") }
	if got := Root(); got != "" {
		t.Errorf("Root() with no executable = %q, want empty", got)
	}
}
