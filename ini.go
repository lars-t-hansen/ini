// Package ini implements a generic, simple ini file parser.
//
// # Syntax
//
// An ini file is line oriented.  It has a number of sections, each starting with a `[section-name]`
// header.  Within each section is a sequence of field settings, each on the form name=value.
// Blank lines are ignored.  Lines whose first nonblank is CommentChar (default `#`) are ignored.
// There can be blanks at the beginning and end of all lines and on either side of the `=`, and
// inside the brackets of the header. Section and field names must conform to `[-a-zA-Z0-9_$]+`, and
// are case-sensitive.
//
// The fields are typed, the value must conform to the type, though blank values are accepted for
// strings (empty string) and booleans (true).  All values can be quoted with matching quotes
// according to QuoteChar (default `"`), the quotes are stripped.  Set QuoteChar to 0 to disable all
// quote stripping.  Leading and trailing blanks of the value (outside any quotes) are always
// stripped.
//
// Environment variable references in the values will be expanded if ExpandVars is true (default
// false).  Variables match the syntax `$[a-zA-Z0-9_]+` or `${[^}]+}`, e.g. `$HOME` or `${HOME AGAIN?}`.
// Variables that are not bound in the environment are replaced by the empty string.  A `$` can be
// doubled to remove its metacharacter meaning: `$$HOME` expands to `$HOME`.  Replacement text is not
// subject to further expansion.  Expansion takes place before blank and quote stripping and value
// interpretation, and is not affected by quoting.
//
// # Usage
//
// Create an ini parser with [NewParser] and customize any variables.  Then add a new [Section] to
// it with [Parser.AddSection].  Add a new [Field] to the section with `Section.Add<Type>()` for
// pre-defined types, eg [Section.AddString], or the general [Section.Add] for user-defined types or
// non-standard default values or parsing.
//
// Parse an input stream with [Parser.Parse].  This will return a [Store] (or an error).  Access
// field values via the Field objects on the Store, or directly on the Store itself.
//
// # Errors
//
// Errors during creation of the parser are considered programming errors and uniformly result in a
// panic.  Errors during parsing are considered input errors and are surfaced as an error return
// from [Parser.Parse].
package ini

import (
	"bufio"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	nameRe = regexp.MustCompile(`^[-a-zA-Z0-9_$]+$`)
	valRe  = regexp.MustCompile(`^\s*([-a-zA-Z0-9_$]+)\s*=(.*)$`)
	varRe  = regexp.MustCompile(`\$\$|\$[a-zA-Z0-9_]+|\$\{[^}]*\}`)
)

// A FieldTy describes the type of the field.
type FieldTy int

const (
	TyString  FieldTy = iota + 1 // The field is a string
	TyBool                       // The field is a bool
	TyInt64                      // The field is an int64
	TyUint64                     // The field is an uint64
	TyFloat64                    // The field is a float64
	TyUser                       // The field is a user-defined type (for this and higher values)
)

// A ParseError describes an error encountered during parsing with its location and nature.
type ParseError struct {
	Line     int    // The line number in the input where the error was discovered
	Section  string // The section name context, if not ""
	Irritant string // Informative text and context
}

func parseFail(line int, section string, format string, args ...any) *ParseError {
	return &ParseError{
		Line:     line,
		Section:  section,
		Irritant: fmt.Sprintf(format, args...),
	}
}

func (pe *ParseError) Error() string {
	if pe.Section != "" {
		return fmt.Sprintf("Line %d: In section %s: %s", pe.Line, pe.Section, pe.Irritant)
	}
	return fmt.Sprintf("Line %d: %s", pe.Line, pe.Irritant)
}

// A Parser holds the structure of the ini file and its parsing options, and performs parsing.
type Parser struct {
	// CommentChar is the character that starts line comments (default '#'): lines whose first
	// nonblank matches CommentChar are stripped from the input.
	CommentChar rune

	// QuoteChar is the character that is used for quoting values (default '"'): values whose first
	// and last nonblank match QuoteChar are stripped of those chars (both must be present for
	// stripping to happen).  Set to 0 to disable quote stripping.
	QuoteChar rune

	// ExpandVars controls the expansion of environment variables in values (default false): if
	// true, environment variable references are replaced by their values.
	ExpandVars bool

	sections map[string]*Section
}

// Make a new, empty parser with default settings.  If options are present they are used to alter
// the settings.  Each option is a pair: a string keyword and a value of the appropriate type.  The
// keywords are the exact option member names, eg, "CommentChar".
func NewParser(options ...any) *Parser {
	p := &Parser{
		CommentChar: '#',
		QuoteChar:   '"',
		ExpandVars:  false,
		sections:    make(map[string]*Section),
	}
	if len(options)%2 != 0 {
		panic("Bad options: must be keyword / value pairs")
	}
	i := 0
	for i < len(options) {
		k := options[i]
		v := options[i+1]
		i += 2
		if kwd, ok := k.(string); ok {
			switch kwd {
			case "CommentChar":
				if val, ok := v.(rune); ok {
					p.CommentChar = val
					continue
				}
			case "QuoteChar":
				if val, ok := v.(rune); ok {
					p.QuoteChar = val
					continue
				}
			case "ExpandVars":
				if val, ok := v.(bool); ok {
					p.ExpandVars = val
					continue
				}
			}
		}
		panic(fmt.Sprintf("Bad keyword / value combination %T %v / %T %v", k, k, v, v))
	}
	return p
}

// AddSection adds a new ini section with the given name to the parser.  A section of that name must
// not be present in the section already, and the name must be syntactically valid (see the package
// documentation).
func (parser *Parser) AddSection(name string) *Section {
	if !nameRe.MatchString(name) {
		panic("Invalid section name " + name)
	}
	if parser.sections[name] != nil {
		panic("Duplicated section name " + name)
	}
	fields := make(map[string]*Field)
	s := &Section{parser, name, fields}
	parser.sections[name] = s
	return s
}

// Section looks up the section by name and returns it if found, otherwise return nil.
func (parser *Parser) Section(name string) *Section {
	return parser.sections[name]
}

// A Section is a named container for a set of fields.
type Section struct {
	parser *Parser
	name   string
	fields map[string]*Field
}

// AddBool adds a new boolean field of the given name to the section.  The name must not be present
// in the section and must be syntactically valid (see package comments).  ParseBool describes the
// accepted values.  The default value is false.
func (section *Section) AddBool(name string) *Field {
	return section.Add(name, TyBool, false, ParseBool)
}

// ParseBool accepts any string representing a bool value, returning the value and a validity flag.
// "true" and the empty string are true values, "false" is the false value.
func ParseBool(s string) (any, bool) {
	switch s {
	case "true", "":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

// AddString adds a new string field of the given name to the section.  The name must not be present
// in the section and must be syntactically valid (see package comments).  ParseString describes the
// accepted values.  The default value is the empty string.
func (section *Section) AddString(name string) *Field {
	return section.Add(name, TyString, "", ParseString)
}

// ParseString accepts any string value, returning its input and true.
func ParseString(s string) (any, bool) {
	return s, true
}

// AddInt64 adds a new int64 field of the given name to the section.  The name must not be present
// in the section and must be syntactically valid (see package comments).  ParseInt64 describes the
// accepted values.  The default value is zero.
func (section *Section) AddInt64(name string) *Field {
	return section.Add(name, TyInt64, int64(0), ParseInt64)
}

// ParseInt64 accepts any string representing a signed, decimal integer in the range of int64,
// returning the value and a validity flag.
func ParseInt64(s string) (any, bool) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// AddUint64 adds a new uint64 field of the given name to the section.  The name must not be present
// in the section and must be syntactically valid (see package comments).  ParseUint64 describes the
// accepted values.  The default value is zero.
func (section *Section) AddUint64(name string) *Field {
	return section.Add(name, TyUint64, uint64(0), ParseUint64)
}

// ParseUint64 accepts any string representing an unsigned, decimal integer in the range of uint64,
// returning the value and a validity flag.
func ParseUint64(s string) (any, bool) {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// AddFloat64 adds a new float64 field of the given name to the section.  The name must not be
// present in the section and must be syntactically valid (see package comments).  ParseFloat64
// describes the accepted values.  The default value is zero.
func (section *Section) AddFloat64(name string) *Field {
	return section.Add(name, TyFloat64, 0.0, ParseFloat64)
}

// ParseFloat64 accepts any string representing a signed, decimal floating-point value in the range
// of float64, returning the value and a validity flag.
func ParseFloat64(s string) (any, bool) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0.0, false
	}
	return v, true
}

// Add adds a field of the given name to the section.  The name must not be present in the section
// and must be syntactically valid (see package comments).  The defaultValue will be used if the
// field is not present in the input.  The ty can be a pre-defined type tag if that is the
// representation of the value, or it must be >= TyUser to indicate something non-standard.  The
// valid function will take a string and return a parsed value and true if the value is good,
// otherwise an arbitrary value and false.
//
// The defaultValue and the value returned by valid must be of the same type, and if a pre-defined
// type tag is used they must both be of the corresponding type.  (A common error is to pass eg 1
// rather than uint64(1) as a defaultValue with TyUint64 for ty.)
func (section *Section) Add(
	name string,
	ty FieldTy,
	defaultValue any,
	valid func(s string) (any, bool),
) *Field {
	if !nameRe.MatchString(name) {
		panic("Invalid field name " + name)
	}
	if ty < 1 {
		panic("Invalid type value")
	}
	if section.fields[name] != nil {
		panic("Duplicated field name " + name + " in section " + section.name)
	}
	f := &Field{section, name, ty, defaultValue, valid}
	section.fields[name] = f
	return f
}

// Name returns the name of the section.
func (section *Section) Name() string {
	return section.name
}

// Field returns the field of the given name from the section, or nil if there is no such field.
func (section *Section) Field(name string) *Field {
	return section.fields[name]
}

// Present returns true if the section was present in the input (even if it contained no settings).
func (section *Section) Present(store *Store) bool {
	return store.lookupSect(section)
}

// A field represents a field within a Section and is also an accessor for the parsed value of that
// field within a Store.
type Field struct {
	section      *Section
	name         string
	ty           FieldTy
	defaultValue any
	valid        func(s string) (any, bool)
}

// Name returns the field's name.
func (field *Field) Name() string {
	return field.name
}

// Type returns the field's type tag.
func (field *Field) Type() FieldTy {
	return field.ty
}

// Present returns true if the field was present in the input.
func (field *Field) Present(store *Store) bool {
	_, found := store.lookupVal(field.section, field)
	return found
}

// BoolVal returns a boolean field's value in the input, or the default if the field was not
// present.
func (field *Field) BoolVal(store *Store) bool {
	if field.ty != TyBool {
		panic("Bool accessor on non-bool field")
	}
	if v, found := store.lookupVal(field.section, field); found {
		return v.(bool)
	}
	return field.defaultValue.(bool)
}

// StringVal returns a string field's value in the input, or the default if the field was not
// present.
func (field *Field) StringVal(store *Store) string {
	if field.ty != TyString {
		panic("String accessor on non-string field")
	}
	if v, found := store.lookupVal(field.section, field); found {
		return v.(string)
	}
	return field.defaultValue.(string)
}

// Float64Val returns a float64 field's value in the input, or the default if the field was not
// present.
func (field *Field) Float64Val(store *Store) float64 {
	if field.ty != TyFloat64 {
		panic("Float64 accessor on non-float64 field")
	}
	if v, found := store.lookupVal(field.section, field); found {
		return v.(float64)
	}
	return field.defaultValue.(float64)
}

// Int64Val returns an int64 field's value in the input, or the default if the field was not
// present.
func (field *Field) Int64Val(store *Store) int64 {
	if field.ty != TyInt64 {
		panic("Int64 accessor on non-int64 field")
	}
	if v, found := store.lookupVal(field.section, field); found {
		return v.(int64)
	}
	return field.defaultValue.(int64)
}

// Uint64Val returns an uint64 field's value in the input, or the default if the field was not
// present.
func (field *Field) Uint64Val(store *Store) uint64 {
	if field.ty != TyUint64 {
		panic("Uint64 accessor on non-uint64 field")
	}
	if v, found := store.lookupVal(field.section, field); found {
		return v.(uint64)
	}
	return field.defaultValue.(uint64)
}

// Value returns field's value in the input as an any, or the default value if the field was not
// present.
func (field *Field) Value(store *Store) any {
	v, found := store.lookupVal(field.section, field)
	if found {
		return v
	}
	return field.defaultValue
}

// A Store holds the result of a successful parse.  It is passed as an argument to methods on
// individual Fields to retrieve those fields' values.
type Store struct {
	sections map[string]*sectStore
}

type sectStore struct {
	values map[string]any
}

func (store *Store) lookupSect(section *Section) bool {
	return store.sections[section.name] != nil
}

func (store *Store) lookupVal(section *Section, field *Field) (any, bool) {
	if sProbe := store.sections[section.name]; sProbe != nil {
		if valProbe, found := sProbe.values[field.name]; found {
			return valProbe, true
		}
	}
	return false, false
}

func (store *Store) ensure(section *Section) *sectStore {
	sProbe := store.sections[section.name]
	if sProbe == nil {
		sProbe = &sectStore{
			values: make(map[string]any),
		}
		store.sections[section.name] = sProbe
	}
	return sProbe
}

func (store *Store) set(section *Section, field *Field, val any) {
	store.ensure(section).values[field.name] = val
}

// Parse parses the input from the reader, returning a [Store] with information about field presence
// and values.  Errors in field parsing result in a [*ParseError] being returned with no store.
// Concurrent parsing is safe, but no sections or fields may be added while the parser is in use for
// parsing in any goroutine.
func (parser *Parser) Parse(r io.Reader) (*Store, error) {
	names := slices.Collect(maps.Keys(parser.sections))
	sectionRe := regexp.MustCompile(`^\s*\[\s*(` + strings.Join(names, "|") + `)\s*\]\s*$`)
	blankRe := regexp.MustCompile(fmt.Sprintf(`^\s*(:?\x{%x}.*)?$`, parser.CommentChar))

	store := &Store{
		sections: make(map[string]*sectStore),
	}
	scanner := bufio.NewScanner(r)
	var lineno int
	var sect *Section
	for scanner.Scan() {
		l := scanner.Text()
		lineno++
		if blankRe.MatchString(l) {
			continue
		}
		if m := sectionRe.FindStringSubmatch(l); m != nil {
			probe := parser.sections[m[1]]
			if probe == nil {
				return nil, parseFail(lineno, "", "Undefined section %s", m[1])
			}
			sect = probe
			store.ensure(sect)
			continue
		}
		if m := valRe.FindStringSubmatch(l); m != nil {
			if sect == nil {
				return nil, parseFail(lineno, "", "Setting %s outside section", m[1])
			}
			field := sect.fields[m[1]]
			if field == nil {
				return nil, parseFail(lineno, sect.name, "No field %s", m[1])
			}
			s := m[2]
			if parser.ExpandVars {
				s = varRe.ReplaceAllStringFunc(s, func(m string) string {
					if m == "$$" {
						return "$"
					}
					var name string
					if m[1] == '{' {
						name = m[2 : len(m)-1]
					} else {
						name = m[1:]
					}
					return os.Getenv(name)
				})
			}
			s = strings.TrimSpace(s)
			if parser.QuoteChar != 0 {
				c := string(parser.QuoteChar)
				if strings.HasPrefix(s, c) && strings.HasSuffix(s, c) {
					s = strings.TrimSuffix(strings.TrimPrefix(s, c), c)
				}
			}
			val, valid := field.valid(s)
			if !valid {
				return nil, parseFail(
					lineno, sect.name, "Value '%s' is not valid for field %s", s, m[1])
			}
			store.set(sect, field, val)
			continue
		}
		if sect == nil {
			return nil, parseFail(lineno, "", "Invalid syntax before first section")
		}
		return nil, parseFail(lineno, sect.name, "Invalid syntax")
	}
	if err := scanner.Err(); err != nil {
		return nil, parseFail(lineno, "", "I/O error: "+err.Error())
	}

	return store, nil
}
