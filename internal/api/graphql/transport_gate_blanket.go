//go:build transportgateblanket

package graphql

// transportGateBlanketAllow restores the pre-Sprint-4 transport gate: every
// authenticated caller is allowed every root field, exactly as
// MutateOperationContext behaved before Lane S4-E replaced the blanket
// graphql:operator allow with rootFieldRoles.
//
// This is the negative control for the transport-gate kill-test, required by
// .loom/32-sprint4-execution-specs.md:467. Building the tests with
//
//	go test -tags transportgateblanket ./internal/api/graphql/
//
// must make every least-privilege refusal FAIL OPEN. A kill-test that still
// passes with this tag is not measuring the narrowing, and the tag exists so
// that claim can be checked rather than asserted.
//
// This tag is never set in a shipped build or in any non-negative-control CI
// job. `make test-transport-gate-negative-control` is the only caller.
func transportGateBlanketAllow() bool { return true }
