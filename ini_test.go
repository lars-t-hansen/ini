// TODO: user types in a better way (try string list)
// TODO: error cases
// TODO: more identifier chars
// TODO: case sensitivity
// TODO: QuoteChar == 0

package ini

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestGood(t *testing.T) {
	p := NewParser()

	sStrings := p.AddSection("strings")
	if p.Section("strings") != sStrings {
		t.Fatal("Lookup section")
	}
	if p.Section("zappa") != nil {
		t.Fatal("Weird section")
	}

	sf := sStrings.AddString("s")
	if sf != sStrings.Field("s") {
		t.Fatal("Lookup field")
	}
	if sStrings.Field("zappa") != nil {
		t.Fatal("Weird field")
	}
	if sf.Name() != "s" {
		t.Fatal("Name")
	}
	if sf.Type() != TyString {
		t.Fatal("Type")
	}

	sStrings.AddString("empty")

	sNums := p.AddSection("nums")
	inf := sNums.AddInt64("i")
	if inf.Type() != TyInt64 {
		t.Fatal("Type")
	}
	uf := sNums.AddUint64("u")
	if uf.Type() != TyUint64 {
		t.Fatal("Type")
	}
	ff := sNums.AddFloat64("f")
	if ff.Type() != TyFloat64 {
		t.Fatal("Type")
	}

	sBools := p.AddSection("bools")
	bf := sBools.AddBool("b")
	if bf.Type() != TyBool {
		t.Fatal("Type")
	}
	sBools.AddBool("implicit")

	sUser := p.AddSection("user-defined-types")
	u1 := sUser.Add("vowel", TyBool, false, func(s string) (any, bool) {
		switch s {
		case "a", "e", "i", "o", "y":
			return true, true
		default:
			return false, len(s) == 1
		}
	})
	if u1.Name() != "vowel" {
		t.Fatal("vowel name")
	}
	if u1.Type() != TyBool {
		t.Fatal("vowel type")
	}
	u2 := sUser.Add("consonant", TyBool, false, func(s string) (any, bool) {
		switch s {
		case "a", "e", "i", "o", "y":
			return false, true
		default:
			if len(s) != 1 {
				return false, false
			}
			return s >= "a" && s <= "z", true
		}
	})
	if u2.Type() != TyBool {
		t.Fatal("consonant type")
	}

	sOther := p.AddSection("other")
	sEmpty := p.AddSection("empty")

	f, err := os.Open("testdata/simple.ini")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var store *Store
	store, err = p.Parse(f)
	if err != nil {
		t.Fatal(err)
	}

	if x := sStrings.Field("s").StringVal(store); x != "hi there" {
		t.Fatal("s: ", x)
	}
	if x := sStrings.Field("empty").StringVal(store); x != "" {
		t.Fatal("empty: ", x)
	}
	if x := inf.Int64Val(store); x != -12 {
		t.Fatal("i: ", x)
	}
	if x := uf.Uint64Val(store); x != 17 {
		t.Fatal("u: ", x)
	}
	if x := ff.Float64Val(store); x != 13.5e+7 {
		t.Fatal("f: ", x)
	}
	if x := ff.Present(store); !x {
		t.Fatal("f: ", x)
	}
	if x := bf.BoolVal(store); x {
		t.Fatal("b: ", x)
	}
	if x := sBools.Field("implicit").BoolVal(store); !x {
		t.Fatal("implicit: ", x)
	}
	if x := sUser.Present(store); !x {
		t.Fatal("user-defined-types present")
	}
	if x := u1.BoolVal(store); !x {
		t.Fatal("vowel: ", x)
	}
	if x := u2.BoolVal(store); !x {
		t.Fatal("consonant: ", x)
	}
	if x := sOther.Present(store); x {
		t.Fatal("other is present")
	}
	if x := sEmpty.Present(store); !x {
		t.Fatal("empty is not present")
	}
}

// '(' is a regexp metachar so this test tests that it is escaped properly in the parser in addition
// to testing that custom comment chars work at all.

func TestComment(t *testing.T) {
	p := NewParser()
	s := p.AddSection("sect")
	s.AddInt64("x")
	s.AddInt64("y")
	p.CommentChar = '('
	store, err := p.Parse(strings.NewReader(`
( this is a comment
( this too
[ sect ]
x = 10
( more comment, next line is blank

y = 20
`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Field("x").Int64Val(store) != 10 {
		t.Fatal("x")
	}
	if s.Field("y").Int64Val(store) != 20 {
		t.Fatal("y")
	}

	// Comment char can be changed right before parse, even after a previous parse
	p.CommentChar = ';'
	store, err = p.Parse(strings.NewReader(`
; this is a comment
[ sect ]
x = 12
`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Field("x").Int64Val(store) != 12 {
		t.Fatal("x")
	}
}

func TestQuote(t *testing.T) {
	p := NewParser()
	s := p.AddSection("sect")
	s.AddInt64("x")
	s.AddString("s")
	store, err := p.Parse(strings.NewReader(`
[ sect ]
x = "10"
s = "hi there"
`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Field("x").Int64Val(store) != 10 {
		t.Fatal("x")
	}
	if s.Field("s").StringVal(store) != "hi there" {
		t.Fatal("s")
	}

	p.QuoteChar = '/'
	store, err = p.Parse(strings.NewReader(`
[ sect ]
x = /10/
s = "hi there"
`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Field("x").Int64Val(store) != 10 {
		t.Fatal("x")
	}
	if s.Field("s").StringVal(store) != `"hi there"` {
		t.Fatal("s")
	}
}

// Non-standard defaults and parsers for pre-defined types

func TestBuiltinDefaultAndParse(t *testing.T) {
	sParse := func(s string) (any, bool) {
		if s == "" {
			return "empty", true
		}
		before, _, _ := strings.Cut(s, " ")
		return before, true
	}
	p := NewParser()
	s := p.AddSection("sect")
	s.Add("x", TyInt64, int64(1), ParseInt64)
	s.Add("s", TyString, "hi", ParseString)
	s.Add("w", TyString, "", sParse)
	s.Add("y", TyString, "", sParse)
	store, err := p.Parse(strings.NewReader(`
[ sect ]
y=
w= ho there 
`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Field("x").Int64Val(store) != 1 {
		t.Fatal("x")
	}
	if s.Field("s").StringVal(store) != "hi" {
		t.Fatal("s")
	}
	if s.Field("y").StringVal(store) != "empty" {
		t.Fatal("y")
	}
	if s.Field("w").StringVal(store) != "ho" {
		t.Fatal("w")
	}
}

func TestOptions(t *testing.T) {
	p := NewParser("CommentChar", ';', "QuoteChar", '/')
	if p.CommentChar != ';' {
		t.Fatal("CommentChar")
	}
	if p.QuoteChar != '/' {
		t.Fatal("QuoteChar")
	}
}

func TestVar(t *testing.T) {
	p := NewParser("ExpandVars", true)
	s := p.AddSection("sect")
	s.AddString("s")
	s.AddString("m")
	s.AddInt64("n")
	os.Setenv("Q", "\"")
	os.Setenv("S", " ")
	store, err := p.Parse(strings.NewReader(`
[ sect ]
s = "hi there $SHELL$SHUL$$${USER}"
m = $Q${S}hello hello $Q
n = ${S}37$S
`))
	if err != nil {
		t.Fatal(err)
	}
	shell := os.Getenv("SHELL")
	user := os.Getenv("USER")
	if s.Field("s").StringVal(store) != "hi there "+shell+"$"+user {
		t.Fatal(s.Field("s").StringVal(store))
	}
	if s.Field("m").StringVal(store) != " hello hello " {
		t.Fatal(s.Field("m").StringVal(store))
	}
	if s.Field("n").Int64Val(store) != 37 {
		t.Fatal(s.Field("n").Int64Val(store))
	}
}

func TestList(t *testing.T) {
	p := NewParser()
	s := p.AddSection("sect")
	s.AddStringList("names")
	s.AddFloat64List("factors")
	store, err := p.Parse(strings.NewReader(`
[sect]
factors=10
factors=20
factors=15.5
factors="12.75"
`))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.Field("factors").Float64ListVal(store), []float64{10, 20, 15.5, 12.75}) {
		t.Fatal(s.Field("factors").Float64ListVal(store))
	}
}
