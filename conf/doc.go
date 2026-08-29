// Package conf provides a small, dependency-free configuration format for
// cfggo. Documents contain key=value assignments and optional [section]
// headers. Values before the first section and values in [global] belong to the
// root configuration object.
//
// A # starts a comment wherever it appears on a line. To include a literal # in
// a quoted string, write it as the JSON escape \u0023.
//
// File handlers are read-only by default because rewriting cannot preserve
// comments or formatting. Pass RewriteOnSave to opt into deterministic output.
package conf
