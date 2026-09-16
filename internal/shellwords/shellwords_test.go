package shellwords

import (
	"reflect"
	"testing"
)

func TestSplit(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"podman run nginx", []string{"podman", "run", "nginx"}},
		{"podman run --name 'my web' nginx", []string{"podman", "run", "--name", "my web", "nginx"}},
		{`podman run -e "A=b c" nginx`, []string{"podman", "run", "-e", "A=b c", "nginx"}},
		{`echo "a\"b"`, []string{"echo", `a"b`}},
		{`echo 'a"b'`, []string{"echo", `a"b`}},
		{`echo a\ b`, []string{"echo", "a b"}},
		{"  spaced\tout  ", []string{"spaced", "out"}},
		{"''", []string{""}},
	}
	for _, tc := range cases {
		got, err := Split(tc.in)
		if err != nil {
			t.Errorf("Split(%q) error: %v", tc.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Split(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSplitErrors(t *testing.T) {
	for _, in := range []string{`"unclosed`, `'unclosed`, `trailing\`} {
		if _, err := Split(in); err == nil {
			t.Errorf("Split(%q) should fail", in)
		}
	}
}
