// TODO doc:
// - conformant documentation
// - examples

// Package ini implements a generic, simple ini file parser.
//
// # Syntax
//
// An ini file is line oriented.  It has a number of sections, each starting with a `[section-name]`
// header.  Within each section is a sequence of field settings, each on the form `name=value`.
// Blank lines are ignored.  Lines whose first nonblank is CommentChar (default '#') are ignored.
// There can be blanks at the beginning and end of all lines and on either side of the `=`, and
// inside the braces of the header. Section and field names must conform to `[a-zA-Z0-9-_]+`, and
// are case-sensitive.
//
// The fields are typed, the value must conform to the type, though blank values are accepted for
// strings (empty string) and booleans (true).  Values can be quoted with matching quotes according
// to QuoteChar (default '"'), the quotes are stripped.  Set QuoteChar to ' ' to disable all quote
// stripping.  Leading and trailing blanks of the value (outside any quotes) are always stripped.
//
// # Usage
//
// Create an ini parser with NewParser() and customize any variables.  Then add sections to it with
// AddSection().  Add fields to each section with Add<Type>() for pre-defined types or the general
// Add() for user-defined types or non-standard default values or parsing.
//
// Parse an input stream with the parser's `Parse()` method.  This will return a `Store` (or an
// error).  Access field values via the Field objects on the Store, or directly on the Store itself.
package ini

import (
	"bufio"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	nameRe = regexp.MustCompile(`^[a-zA-Z0-9-_]+$`)
	valRe  = regexp.MustCompile(`^\s*([a-zA-Z0-9-_]+)\s*=(.*)$`)
)

// Type tags for fields.
type FieldTy int

const (
	TyString  FieldTy = iota + 1 // The field is a string
	TyBool                       // The field is a bool
	TyInt64                      // The field is an int64
	TyUint64                     // The field is an uint64
	TyFloat64                    // The field is a float64
	TyUser                       // The field is a user-defined type (for this and higher FieldTy values)
)

// An error from the parser
type ParseError struct {
	Line int					// The line number in the input where the error was discovered
	Section string				// The section name context, if not ""
	Irritant string				// Informative text and context
}

func parseFail(line int, section string, format string, args ...any) *ParseError {
	return &ParseError {
		Line: line,
		Section: section,
		Irritant: fmt.Sprintf(format, args...),
	}
}

func (pe *ParseError) Error() string {
	if pe.Section != "" {
		return fmt.Sprintf("Line %d: In section %s: %s", pe.Line, pe.Section, pe.Irritant)
	}
	return fmt.Sprintf("Line %d: %s", pe.Line, pe.Irritant)
}

// A Parser is a container for a set of sections.
type Parser struct {
	// Lines whose first nonblank matches CommentChar are stripped.
	CommentChar rune
	// Values whose first and last nonblank match QuoteChar are stripped of those chars (both must
	// be present for stripping to happen).  Set to ' ' to disable quote stripping.
	QuoteChar rune
	sections  map[string]*Section
}

// Make a new, empty parser with default settings.
func NewParser() *Parser {
	return &Parser{
		CommentChar: '#',
		QuoteChar:   '"',
		sections:    make(map[string]*Section),
	}
}

// Add a new section with the given name (which must not be present and must be syntactically valid).
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

// Look up the section and return it if found, otherwise return nil.
func (parser *Parser) Section(name string) *Section {
	return parser.sections[name]
}

// A Section is a named container for a set of fields.
type Section struct {
	parser *Parser
	name   string
	fields map[string]*Field
}

// Add a new boolean field of the given name to the section.  Values can be true, false, or the
// empty string (meaning true).
func (section *Section) AddBool(name string) *Field {
	return section.Add(name, TyBool, false, ParseBool)
}

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

// Add a new string field of the given name to the section.  Values can be any string.
func (s *Section) AddString(name string) *Field {
	return s.Add(name, TyString, "", ParseString)
}

func ParseString(s string) (any, bool) {
	return s, true
}

// Add a new int64 field of the given name to the section.  Values can be signed, decimal integers.
func (section *Section) AddInt64(name string) *Field {
	return section.Add(name, TyInt64, int64(0), ParseInt64)
}

func ParseInt64(s string) (any, bool) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// Add a new uint64 field of the given name to the section.  Values can be unsigned, decimal
// integers.
func (section *Section) AddUint64(name string) *Field {
	return section.Add(name, TyUint64, uint64(0), ParseUint64)
}

func ParseUint64(s string) (any, bool) {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// Add a new float64 field of the given name to the section.  Values can be signed, decimal
// floating-point numbers.
func (s *Section) AddFloat64(name string) *Field {
	return s.Add(name, TyFloat64, 0.0, ParseFloat64)
}

func ParseFloat64(s string) (any, bool) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0.0, false
	}
	return v, true
}

// Add a field of the given name and default value to the section.  The `ty` can be a pre-defined
// type tag if that is the representation of the value, or it must be >= TyUser to indicate
// something non-standard.  The `valid` function will take a string and return a parsed value and
// true if the value is good, otherwise an arbitrary value and false.
func (section *Section) Add(name string, ty FieldTy, defaultValue any, valid func(s string) (any, bool)) *Field {
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

// Return the field from the section, or nil if there is no such field.
func (s *Section) Field(name string) *Field {
	return s.fields[name]
}

// Return true if the section was present in the input (even if it contained no settings).
func (section *Section) Present(store *Store) bool {
	return store.lookupSect(section)
}

// A field represent both a field within a Section and is an accessor for the parsed value of that
// field within a Store.
type Field struct {
	section      *Section
	name         string
	ty           FieldTy
	defaultValue any
	valid        func(s string) (any, bool)
}

// The field's name.
func (field *Field) Name() string {
	return field.name
}

// The field's type tag.
func (field *Field) Type() FieldTy {
	return field.ty
}

// Return true if the field was present in the input.
func (f *Field) Present(s *Store) bool {
	_, found := s.lookupVal(f.section, f)
	return found
}

// Return the field's value in the input, or false.
func (f *Field) BoolVal(s *Store) bool {
	if f.ty != TyBool {
		panic("Bool accessor on non-bool field")
	}
	if v, found := s.lookupVal(f.section, f); found {
		return v.(bool)
	}
	return f.defaultValue.(bool)
}

// Return the field's value in the input, or the empty string.
func (f *Field) StringVal(s *Store) string {
	if f.ty != TyString {
		panic("String accessor on non-string field")
	}
	if v, found := s.lookupVal(f.section, f); found {
		return v.(string)
	}
	return f.defaultValue.(string)
}

// Return the field's value in the input, or zero.
func (f *Field) Float64Val(s *Store) float64 {
	if f.ty != TyFloat64 {
		panic("Float64 accessor on non-float64 field")
	}
	if v, found := s.lookupVal(f.section, f); found {
		return v.(float64)
	}
	return f.defaultValue.(float64)
}

// Return the field's value in the input, or zero.
func (f *Field) Int64Val(s *Store) int64 {
	if f.ty != TyInt64 {
		panic("Int64 accessor on non-int64 field")
	}
	if v, found := s.lookupVal(f.section, f); found {
		return v.(int64)
	}
	return f.defaultValue.(int64)
}

// Return the field's value in the input, or zero.
func (f *Field) Uint64Val(s *Store) uint64 {
	if f.ty != TyUint64 {
		panic("Uint64 accessor on non-uint64 field")
	}
	if v, found := s.lookupVal(f.section, f); found {
		return v.(uint64)
	}
	return f.defaultValue.(uint64)
}

// Return the field's value in the input as an any, or the default value if the field was not
// present.
func (f *Field) Value(s *Store) any {
	v, found := s.lookupVal(f.section, f)
	if found {
		return v
	}
	return f.defaultValue
}

// The Store holds the result of a successful parse.  It is passed as an argument to methods on
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

// Parse the input from the reader.  Concurrent parsing is safe, but no sections or fields may be
// added while parsing is happening in any goroutine.
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
			s := strings.TrimSpace(m[2])
			if parser.QuoteChar != ' ' {
				c := string(parser.QuoteChar)
				if strings.HasPrefix(s, c) && strings.HasSuffix(s, c) {
					s = strings.TrimSuffix(strings.TrimPrefix(s, c), c)
				}
			}
			val, valid := field.valid(s)
			if !valid {
				return nil, parseFail(lineno, sect.name, "Value '%s' is not valid for field %s", s, m[1])
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
		return nil, parseFail(lineno, "", "I/O error: " + err.Error())
	}

	return store, nil
}
