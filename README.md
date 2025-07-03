# ini

Package ini implements a generic, simple ini file parser.

# Syntax

An ini file is line oriented. It has a number of sections, each starting with a
`[section-name]` header. Within each section is a sequence of field settings,
each on the form name=value. Blank lines are ignored. Lines whose first nonblank
is CommentChar (default `#`) are ignored. There can be blanks at the beginning
and end of all lines and on either side of the `=`, and inside the brackets of
the header. Section and field names must conform to `[-a-zA-Z0-9_$]+`, and are
case-sensitive.

The fields are typed, the value must conform to the type, though blank values
are accepted for strings (empty string) and booleans (true). All values can be
quoted with matching quotes according to QuoteChar (default `"`), the quotes
are stripped. Set QuoteChar to 0 to disable all quote stripping. Leading and
trailing blanks of the value (outside any quotes) are always stripped.

Environment variable references in the values will be expanded if ExpandVars is
true (default false). Variables match the syntax `$[a-zA-Z0-9_]+` or `${[^}]+}`,
e.g. `$HOME` or `${HOME AGAIN?}`. Variables that are not bound in the
environment are replaced by the empty string. A `$` can be doubled to remove
its metacharacter meaning: `$$HOME` expands to `$HOME`. Replacement text is
not subject to further expansion. Expansion takes place before blank and quote
stripping and value interpretation, and is not affected by quoting.

# Usage

Create an ini parser with NewParser and customize any variables. Then add a
new Section to it with Parser.AddSection. Add a new Field to the section with
`Section.Add<Type>()` for pre-defined types, eg Section.AddString, or the
general Section.Add for user-defined types or non-standard default values or
parsing.

Parse an input stream with Parser.Parse. This will return a Store (or an error).
Access field values via the Field objects on the Store, or directly on the Store
itself.

# Errors

Errors during creation of the parser are considered programming errors and
uniformly result in a panic. Errors during parsing are considered input errors
and are surfaced as an error return from Parser.Parse.

