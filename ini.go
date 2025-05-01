/*
   Generic, simple ini file parser.

   An ini file is line oriented.  It has a number of sections, each starting with a `[section-name]`
   header.  Within each section is a sequence of fields, each on the form `name=value`.  Blank lines
   are ignored.  Lines whose first nonblank is CommentChar (default '#') are ignored.  There can be
   blanks at the beginning and end of all lines and on either side of the `=`, and inside the braces
   of the header.  Section and field names must conform to `[a-zA-Z0-9-_]+`, and are case-sensitive.

   The fields are typed, the value must conform to the type.  Values can be quoted with matching
   quotes according to QuoteChar (default '"'), the quotes are stripped.  Set QuoteChar to ' ' to
   disable all quote stripping.  Leading and trailing blanks of the value (outside any quotes) are
   always stripped.

   Create an ini parser with `NewIniParser()` and customize any variables.  Then add sections to it
   with `AddSection()`.  Add fields to each section with `AddField<Type>()`.

   Parse an input stream with the parser's `Parse()` method.  This will return a `Store` (or an
   error).  Access field values via the Field objects on the Store, or directly on the Store itself.
 */
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
	valRe = regexp.MustCompile(`^\s*([a-zA-Z0-9-_]+)\s*=(.*)$`)
)

type FieldTy int

const (
	// Field is a string
	TyString FieldTy = iota+1
	// Field is a bool
	TyBool
	// Field is an int64
	TyInt64
	// Field is an uint64
	TyUint64
	// Field is a float64
	TyFloat64
	// Field is a user-defined type
	TyUser
)

// The IniParser is a container for a set of sections.
type IniParser struct {
	// Lines whose first nonblank matches CommentChar are stripped.
	CommentChar rune

	// Values whose first and last nonblank match QuoteChar are stripped of those chars (both must
	// be present for stripping to happen).  Set to ' ' to disable quote stripping.
	QuoteChar   rune

	sections    map[string]*Section
}

func NewIniParser() *IniParser {
	return &IniParser{
		CommentChar: '#',
		QuoteChar:   '"',
		sections:    make(map[string]*Section),
	}
}

// Add a new section with the given name (which must not be present and must be syntactically valid).
func (parser *IniParser) AddSection(name string) *Section {
	if !nameRe.MatchString(name) {
		panic("Invalid section name " + name)
	}
	if parser.sections[name] != nil {
		panic("Duplicated section name " + name)
	}
	fields := make(map[string]*Field)
	s := &Section{ parser, name, fields }
	parser.sections[name] = s
	return s
}

// Lookup the section and return it, or return nil.
func (parser *IniParser) Section(name string) *Section {
	return parser.sections[name]
}

// A container for a set of fields.
type Section struct {
	parser   *IniParser
	name   string
	fields map[string]*Field
}

func (section *Section) addField(name string, ty FieldTy, valid func(string) (any, bool)) *Field {
	if !nameRe.MatchString(name) {
		panic("Invalid field name " + name)
	}
	if section.fields[name] != nil {
		panic("Duplicated field name " + name + " in section " + section.name)
	}
	f := &Field{ section, name, ty, valid }
	section.fields[name] = f
	return f
}

// Add a new boolean field of the given name.  Values can be true, false, or the empty string
// (meaning true).
func (section *Section) AddBool(name string) *Field {
	return section.addField(name, TyBool, func (s string) (any, bool) {
		switch s {
		case "true", "":
			return true, true
		case "false":
			return false, true
		default:
			return false, false
		}
	})
}

// Add a new string field of the given name.
func (s *Section) AddString(name string) *Field {
	return s.addField(name, TyString, func (s string) (any, bool) {
		return s, true
	})
}

// Add a new int64 field of the given name.
func (section *Section) AddInt64(name string) *Field {
	return section.addField(name, TyInt64, func (s string) (any, bool) {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, false
		}
		return v, true
	})
}

// Add a new uint64 field of the given name.
func (section *Section) AddUint64(name string) *Field {
	return section.addField(name, TyUint64, func (s string) (any, bool) {
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return 0, false
		}
		return v, true
	})
}

// Add a new float64 field of the given name.
func (s *Section) AddFloat64(name string) *Field {
	return s.addField(name, TyFloat64, func (s string) (any, bool) {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0.0, false
		}
		return v, true
	})
}

// Add a custom (user-defined) field.  The ty is uinterpreted but must be >= TyUser.  The valid
// function will take a string and return a parsed value and true if the value is good, otherwise
// an arbitrary value and false.
func (s *Section) Add(name string, ty FieldTy, valid func (s string) (any, bool)) *Field {
	if ty < TyUser {
		panic("Invalid user-defined type value")
	}
	return s.addField(name, ty, valid)
}

func (s *Section) Field(name string) *Field {
	return s.fields[name]
}

// A field represent both a field within a Section and is an accessor for the parsed value of that
// field within a Store.
type Field struct {
	section *Section
	name    string
	ty      FieldTy
	valid   func(s string) (any, bool)
}

func (field *Field) Name() string {
	return field.name
}

func (field *Field) Type() FieldTy {
	return field.ty
}

func (f *Field) Present(s *Store) bool {
	_, found := s.lookup(f.section, f)
	return found
}

func (f *Field) BoolVal(s *Store) bool {
	if f.ty != TyBool {
		panic("Bool accessor on non-bool field")
	}
	if v, found := s.lookup(f.section, f); found {
		return v.(bool)
	}
	return false
}

func (f *Field) StringVal(s *Store) string {
	if f.ty != TyString {
		panic("String accessor on non-string field")
	}
	if v, found := s.lookup(f.section, f); found {
		return v.(string)
	}
	return ""
}

func (f *Field) Float64Val(s *Store) float64 {
	if f.ty != TyFloat64 {
		panic("Float64 accessor on non-float64 field")
	}
	if v, found := s.lookup(f.section, f); found {
		return v.(float64)
	}
	return 0.0
}

func (f *Field) Value(s *Store) (any, bool) {
	return s.lookup(f.section, f)
}

// The Store holds the result of a successful parse.  It is passed as an argument to methods on
// individual Fields to retrieve those fields' values.
type Store struct {
	sections map[string]*sectStore
}

type sectStore struct {
	values map[string]any
}

func (store *Store) lookup(section *Section, field *Field) (any, bool) {
	if sProbe := store.sections[section.name]; sProbe != nil {
		if valProbe, found := sProbe.values[field.name]; found {
			return valProbe, true
		}
	}
	return false, false
}

func (store *Store) set(section *Section, field *Field, val any) {
	sProbe := store.sections[section.name]
	if sProbe == nil {
		sProbe = &sectStore{
			values: make(map[string]any),
		}
		store.sections[section.name] = sProbe
	}
	sProbe.values[field.name] = val
}

// Parse the input from the reader.
func (parser *IniParser) Parse(r io.Reader) (*Store, error) {
	names := slices.Collect(maps.Keys(parser.sections))
	sectionRe := regexp.MustCompile(`^\s*\[\s*(` + strings.Join(names, "|") + `)\s*\]\s*$`)
	// FIXME: Must escape that char somehow
	blankRe := regexp.MustCompile(fmt.Sprintf(`^\s*(:?%c.*)?$`, parser.CommentChar))

	store := &Store {
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
				return nil, fmt.Errorf("Line %d: Undefined section %s", lineno, m[1])
			}
			sect = probe
			continue
		}
		if m := valRe.FindStringSubmatch(l); m != nil {
			if sect == nil {
				return nil, fmt.Errorf("Line %d: Setting %s outside section", lineno, m[1])
			}
			field := sect.fields[m[1]]
			if field == nil {
				return nil, fmt.Errorf("Line %d: No field %s in section %s", lineno, m[1], sect.name)
			}
			s := strings.TrimSpace(m[2])
			if parser.QuoteChar != ' ' {
				// TODO: Quote stripping, note we may need to do the whole rune, and only if both
				// first and last match.
			}
			val, valid := field.valid(s)
			if !valid {
				return nil, fmt.Errorf("Line %d: Value '%s' is not valid for field %s of section %s", lineno, s, m[1], sect.name)
			}
			store.set(sect, field, val)
			continue
		}
		return nil, fmt.Errorf("Line %d: invalid syntax", lineno)
	}
	// TODO: check scanner error

	return store, nil
}
