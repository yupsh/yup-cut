package main

import (
	"context"
	"errors"
	"strconv"
	"testing"

	clix "github.com/gloo-foo/cli"
	command "github.com/gloo-foo/cmd-cut"
	"github.com/spf13/afero"
	urf "github.com/urfave/cli/v3"
)

// parse runs args through a bare command carrying the wrapper's flags and
// returns the parsed accessor, so flag-dependent helpers are tested against real
// parsed flags.
func parse(t *testing.T, args ...string) *urf.Command {
	t.Helper()
	var got *urf.Command
	app := &urf.Command{
		Name:   name,
		Flags:  flags(),
		Action: func(_ context.Context, c *urf.Command) error { got = c; return nil },
	}
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("parse: %v", err)
	}
	return got
}

func TestOptions(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"none", []string{name}, 0},
		{"delimiter", []string{name, "-d", ":"}, 1},
		{"chars", []string{name, "-c", "1-3"}, 1},
		{"bytes", []string{name, "-b", "1-3"}, 1},
		{"complement", []string{name, "--complement"}, 1},
		{"fields", []string{name, "-f", "1,3"}, 1},
		{"all", []string{name, "-d", ":", "-c", "1", "-b", "2", "--complement", "-f", "1-2"}, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := options(parse(t, tc.args...))
			if err != nil {
				t.Fatalf("options err=%v", err)
			}
			if len(opts) != tc.want {
				t.Fatalf("options len=%d, want %d", len(opts), tc.want)
			}
		})
	}
}

func TestOptionsError(t *testing.T) {
	if _, err := options(parse(t, name, "-f", "x")); !errors.Is(err, ErrInvalidFields) {
		t.Fatalf("err=%v, want ErrInvalidFields", err)
	}
}

func TestParseFields(t *testing.T) {
	cases := []struct {
		list string
		want []command.CutField
	}{
		{"1,3", []command.CutField{1, 3}},
		{"5-7", []command.CutField{5, 6, 7}},
		{"-3", []command.CutField{1, 2, 3}},
		{"1,,3", []command.CutField{1, 3}},
		{"2", []command.CutField{2}},
	}
	for _, tc := range cases {
		t.Run(tc.list, func(t *testing.T) {
			got, err := parseFields(fieldList(tc.list))
			if err != nil {
				t.Fatalf("parseFields err=%v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestParseFieldsError(t *testing.T) {
	cases := []struct {
		wantErr error
		list    string
	}{
		{ErrOpenEndedField, "1-"},
		{ErrInvalidFields, "x"},
		{ErrInvalidFields, "x-3"},
		{ErrInvalidFields, "3-x"},
	}
	for _, tc := range cases {
		t.Run(tc.list, func(t *testing.T) {
			if _, err := parseFields(fieldList(tc.list)); !errors.Is(err, tc.wantErr) {
				t.Fatalf("err=%v, want %v", err, tc.wantErr)
			}
		})
	}
}

// TestParseFieldsWrapsCause asserts an invalid numeral's parse failure keeps the
// strconv cause matchable alongside the ErrInvalidFields sentinel.
func TestParseFieldsWrapsCause(t *testing.T) {
	_, err := parseFields(fieldList("x"))
	if !errors.Is(err, ErrInvalidFields) {
		t.Fatalf("err=%v, want ErrInvalidFields", err)
	}
	if !errors.Is(err, strconv.ErrSyntax) {
		t.Fatalf("err=%v, want wrapped strconv.ErrSyntax cause", err)
	}
}

func TestErrorMessage(t *testing.T) {
	if ErrInvalidFields.Error() != string(ErrInvalidFields) {
		t.Fatalf("Error()=%q, want %q", ErrInvalidFields.Error(), string(ErrInvalidFields))
	}
}

func TestBuild(t *testing.T) {
	src, filter, err := build(clix.Invocation{Args: parse(t, name, "-f", "1"), Fs: afero.NewMemMapFs()})
	if err != nil || src == nil || filter == nil {
		t.Fatalf("build: src=%v filter=%v err=%v", src, filter, err)
	}
}

func TestBuildError(t *testing.T) {
	src, filter, err := build(clix.Invocation{Args: parse(t, name, "-f", "1-"), Fs: afero.NewMemMapFs()})
	if !errors.Is(err, ErrOpenEndedField) {
		t.Fatalf("err=%v, want ErrOpenEndedField", err)
	}
	if src != nil || filter != nil {
		t.Fatalf("src=%v filter=%v, want both nil on error", src, filter)
	}
}

func Test_main(t *testing.T) {
	orig := runMain
	t.Cleanup(func() { runMain = orig })
	var gotName clix.Name
	runMain = func(s clix.Spec, _ clix.Version) { gotName = s.Name }
	main()
	if gotName != name {
		t.Fatalf("main used spec %q, want %s", gotName, name)
	}
}
