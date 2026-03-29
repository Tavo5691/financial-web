package parser

import (
	"fmt"
	"sort"
)

var registry = map[string]Parser{}

// Register adds a parser to the global registry.
// Called from each parser package's init() function.
// Panics on duplicate ID — caught at startup, not silently at runtime.
func Register(p Parser) {
	if _, exists := registry[p.ID()]; exists {
		panic(fmt.Sprintf("parser: duplicate registration for ID %q", p.ID()))
	}
	registry[p.ID()] = p
}

// Get retrieves a registered parser by ID.
func Get(id string) (Parser, bool) {
	p, ok := registry[id]
	return p, ok
}

// All returns all registered parsers sorted by ID.
func All() []Parser {
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	parsers := make([]Parser, 0, len(ids))
	for _, id := range ids {
		parsers = append(parsers, registry[id])
	}
	return parsers
}
