package ini

import (
	"os"
	"testing"
)

// TODO: Quotes

func TestGood(t *testing.T) {
	p := NewIniParser()

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
	u1 := sUser.Add("vowel", TyUser, func(s string) (any, bool) {
		switch s {
		case "a","e","i","o","y":
			return true, true
		default:
			return false, len(s) == 1
		}
	})
	if u1.Name() != "vowel" {
		t.Fatal("vowel name")
	}
	if u1.Type() != TyUser {
		t.Fatal("vowel type")
	}
	u2 := sUser.Add("consonant", TyUser+1, func(s string) (any, bool) {
		switch s {
		case "a","e","i","o","y":
			return false, true
		default:
			if len(s) != 1 {
				return false, false
			}
			return s >= "a" && s <= "z", true
		}
	})
	if u2.Type() != TyUser+1 {
		t.Fatal("consonant type")
	}

	f, err := os.Open("testdata/simple.ini")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	store, err := p.Parse(f)
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
}
