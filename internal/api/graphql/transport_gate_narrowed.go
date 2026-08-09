//go:build !transportgateblanket

package graphql

// transportGateBlanketAllow reports whether the pre-Sprint-4 blanket allow is
// compiled in. It is false in every shipped build: the transport gate enumerates
// roots through rootFieldRoles and refuses anything it does not name.
//
// The transportgateblanket build tag selects the other implementation. See
// transport_gate_blanket.go for why that exists.
func transportGateBlanketAllow() bool { return false }
