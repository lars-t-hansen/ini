package ini_test

import (
	"fmt"
	"strings"

	"github.com/lars-t-hansen/ini"
)

func ExampleParser_Parse() {
	p := ini.NewParser()
	p.CommentChar = ';'

	gSect := p.AddSection("global")
	verbose := gSect.AddBool("verbose")

	gUser := p.AddSection("user")
	userName := gUser.AddString("name")
	userLevel := gUser.AddUint64("level")

	store, err := p.Parse(strings.NewReader(`
; hi there
[global]
verbose = true

[user]
 name=Frank
level= 37
`))
	if err != nil {
		panic(err)
	}
	fmt.Printf("global.verbose = %v\n", verbose.BoolVal(store))
	fmt.Printf("user.name = %s\n", userName.StringVal(store))
	fmt.Printf("user.level = %d\n", userLevel.Uint64Val(store))
	// Output:
	// global.verbose = true
	// user.name = Frank
	// user.level = 37
}
