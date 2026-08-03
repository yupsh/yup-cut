// Command yup-cut is the CLI wrapper around github.com/gloo-foo/cmd-cut.
package main

import (
	"strconv"
	"strings"

	clix "github.com/gloo-foo/cli"
	command "github.com/gloo-foo/cmd-cut"
	errs "github.com/gomatic/go-error"
	urf "github.com/urfave/cli/v3"
)

// version is the build version. It defaults to "dev" for local builds and is
// overridden at release time via the linker: -ldflags "-X main.version=<v>".
var version = "dev"

const (
	name           = "cut"
	flagDelimiter  = "delimiter"
	flagFields     = "fields"
	flagChars      = "characters"
	flagBytes      = "bytes"
	flagComplement = "complement"
)

const (
	// ErrInvalidFields reports a malformed -f/--fields list part.
	ErrInvalidFields errs.Const = "invalid field list"
	// ErrOpenEndedField reports a "-f N-" open-ended-to-end field range. The
	// pinned cmd-cut field API selects fields by explicit 1-based position and
	// cannot express an unbounded upper end; -c/-b accept "N-" since they take
	// a spec string. See cmd-cut COMPATIBILITY.md.
	ErrOpenEndedField errs.Const = "open-ended field range (N-) is not supported; list the fields explicitly or use -c/-b"
)

// synopsis is the multi-line --help usage block. urfave/cli indents the whole
// block three spaces, so the lines stay flush-left.
const synopsis = `cut OPTION... [FILE...]

print selected parts of lines from each FILE to standard output.
With no FILE, or when FILE is -, read standard input.`

// spec declares the cut wrapper: a file-or-stdin filter selecting fields, bytes,
// or characters from each line.
var spec = clix.Spec{
	Name:     name,
	Summary:  "remove sections from each line of files",
	Synopsis: synopsis,
	Build:    build,
	Flags:    flags(),
}

// flags builds the wrapper's flag set. It is a constructor rather than a shared
// slice so each urfave/cli command owns distinct flag instances: urfave/cli
// records a flag's "was set" state on the flag value itself and never clears it,
// so reusing one slice across parses would leak IsSet state between them.
func flags() []urf.Flag {
	return []urf.Flag{
		&urf.StringFlag{
			Name:    flagDelimiter,
			Aliases: []string{"d"},
			Usage:   "use DELIM instead of TAB for field delimiter",
			Sources: urf.EnvVars("YUP_CUT_DELIMITER"),
			Value:   "",
		},
		&urf.StringFlag{
			Name:    flagFields,
			Aliases: []string{"f"},
			Usage:   "select only these fields (comma-separated list)",
			Sources: urf.EnvVars("YUP_CUT_FIELDS"),
			Value:   "",
		},
		&urf.StringFlag{
			Name:    flagChars,
			Aliases: []string{"c"},
			Usage:   "select only these characters (e.g. 1-3,5)",
			Sources: urf.EnvVars("YUP_CUT_CHARACTERS"),
			Value:   "",
		},
		&urf.StringFlag{
			Name:    flagBytes,
			Aliases: []string{"b"},
			Usage:   "select only these bytes (e.g. 1-3,5)",
			Sources: urf.EnvVars("YUP_CUT_BYTES"),
			Value:   "",
		},
		&urf.BoolFlag{
			Name:    flagComplement,
			Usage:   "complement the set of selected bytes, characters or fields",
			Sources: urf.EnvVars("YUP_CUT_COMPLEMENT"),
		},
	}
}

// build maps the invocation to cut's pipeline: a file-or-stdin source into the
// cut command configured by the parsed flags. A malformed -f spec is a usage
// error.
func build(inv clix.Invocation) (clix.Source, clix.Command, error) {
	opts, err := options(inv.Args)
	if err != nil {
		return nil, nil, err
	}
	return clix.OperandsOrStdin(inv), command.Cut(opts...), nil
}

// options translates the selected flags into Cut option values. The byte (-b)
// and character (-c) specs are passed through verbatim, since the command
// package parses their full grammar (including open-ended ranges). Fields (-f)
// must be expanded to explicit 1-based positions here, because the command
// package's field API selects by explicit position.
func options(c *urf.Command) ([]any, error) {
	opts := positionOpts(c)
	if c.Bool(flagComplement) {
		opts = append(opts, command.CutComplement)
	}
	if !c.IsSet(flagFields) {
		return opts, nil
	}
	fields, err := parseFields(fieldList(c.String(flagFields)))
	if err != nil {
		return nil, err
	}
	return append(opts, command.CutFields(fields...)), nil
}

// positionOpts collects the delimiter and the verbatim byte/character specs.
func positionOpts(c *urf.Command) []any {
	var opts []any
	if c.IsSet(flagDelimiter) {
		opts = append(opts, command.CutDelimiter(c.String(flagDelimiter)))
	}
	if c.IsSet(flagChars) {
		opts = append(opts, command.CutChars(c.String(flagChars)))
	}
	if c.IsSet(flagBytes) {
		opts = append(opts, command.CutBytes(c.String(flagBytes)))
	}
	return opts
}

// fieldList is the comma-separated -f/--fields LIST of 1-based positions and
// ranges, e.g. "1,3,5-7".
type fieldList string

// parseFields parses a comma-separated list of 1-based positions and ranges
// ("1,3,5-7", "-3") into explicit field positions. Empty parts are skipped, as
// GNU cut tolerates them. An open-ended "N-" range is rejected (ErrOpenEndedField).
func parseFields(list fieldList) ([]command.CutField, error) {
	var result []command.CutField
	for _, part := range strings.Split(string(list), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		nums, err := parsePart(listPart(part))
		if err != nil {
			return nil, err
		}
		result = append(result, nums...)
	}
	return result, nil
}

// listPart is one comma-free part of the field LIST: "N", "N-M", or "-M".
type listPart string

// parsePart parses one comma-free spec part: "N", "N-M", or "-M".
func parsePart(part listPart) ([]command.CutField, error) {
	start, end, found := strings.Cut(string(part), "-")
	if !found {
		n, err := atoi(rangeBound(start))
		return []command.CutField{command.CutField(n)}, err
	}
	return parseRange(rangeBound(start), rangeBound(end))
}

// rangeBound is the textual lower or upper bound of a field range — a 1-based
// position numeral, or empty when the bound is omitted.
type rangeBound string

// parseRange expands a "lo-hi" range into explicit positions. A missing lo
// defaults to 1 ("-M"); a missing hi ("N-") is open-ended and unsupported.
func parseRange(start, end rangeBound) ([]command.CutField, error) {
	if end == "" {
		return nil, ErrOpenEndedField
	}
	lo := command.CutField(1)
	if start != "" {
		parsed, err := atoi(start)
		if err != nil {
			return nil, err
		}
		lo = command.CutField(parsed)
	}
	hi, err := atoi(end)
	if err != nil {
		return nil, err
	}
	return rangePositions(lo, command.CutField(hi)), nil
}

// rangePositions lists the inclusive positions from lo to hi.
func rangePositions(lo, hi command.CutField) []command.CutField {
	var result []command.CutField
	for i := lo; i <= hi; i++ {
		result = append(result, i)
	}
	return result
}

func atoi(bound rangeBound) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(string(bound)))
	if err != nil {
		return 0, ErrInvalidFields.With(err, string(bound))
	}
	return n, nil
}

// runMain is an indirection seam so main's wiring is testable without spawning
// the process; a test swaps it and restores it.
var runMain = clix.Main

func main() { runMain(spec, version) }
