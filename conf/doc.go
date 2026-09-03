// Package conf provides a small, dependency-free configuration format for
// cfggo. Documents contain key=value assignments and optional [section]
// headers. Values before the first section and values in [global] belong to the
// root configuration object.
//
// A value is text unless it is a JSON literal: a quoted string, an array, an
// object, null, true, false or a number. So port = 8080 is the number 8080 and
// name = api is the string "api"; quote a value (version = "1.0") to keep a
// number-like literal as text. Encode writes JSON literals, so a rewritten
// document decodes to the same types it was encoded from.
//
// A # starts a comment wherever it appears on a line. To include a literal # in
// a quoted string, write it as the JSON escape \u0023.
//
// File handlers are read-only by default because rewriting cannot preserve
// comments or formatting. Pass RewriteOnSave to opt into deterministic output.
package conf
