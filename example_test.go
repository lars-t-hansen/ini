package ini_test

import (
	"fmt"
	"strings"

	"github.com/lars-t-hansen/ini"
)

func Example() {
	p := ini.NewParser()
	p.CommentChar = ';'

	sGlobal := p.AddSection("global")
	globalVerbose := sGlobal.AddBool("verbose")

	sUser := p.AddSection("user")
	userName := sUser.AddString("name")
	userLevel := sUser.AddUint64("level")
	userFactors := sUser.AddFloat64List("factors")

	store, err := p.Parse(strings.NewReader(`
; hi there
[global]
verbose = true

[user]
 name=Frank
level= 37
factors = [
# Initially easy
10, 20,

# But gradually much harder
23.5, "38.25",
]
`))
	if err != nil {
		panic(err)
	}
	fmt.Printf("global.verbose = %v\n", globalVerbose.BoolVal(store))
	fmt.Printf("user.name = %s\n", userName.StringVal(store))
	fmt.Printf("user.level = %d\n", userLevel.Uint64Val(store))
	fmt.Printf("user.factors = %v\n", userFactors.Float64ListVal(store))
	// Output:
	// global.verbose = true
	// user.name = Frank
	// user.level = 37
	// user.factors = [10,20,23.5,38.25]
}
