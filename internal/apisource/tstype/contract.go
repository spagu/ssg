// Package tstype parses a type expression — TypeScript syntax, which also
// covers the type expressions of JSDoc comments — into an apimodel.TypeRef
// tree (GO-105, used by GO-104).
//
// This file is the contract; the implementation fills it.
package tstype

// Resolver maps a type name as written ("Token", "ns.Token") to the ID of a
// documented symbol, or "" when it is not one (built-ins, outside types).
type Resolver func(name string) string
