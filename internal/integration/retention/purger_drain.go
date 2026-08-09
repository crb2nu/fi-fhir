//go:build !retentionnodrain

package retention

// drainOnFullBatch reports whether one purge tick keeps going while a bounded
// statement comes back full. It is true in every shipped build: that is the D1
// repair.
//
// The retentionnodrain build tag selects the other implementation. See
// purger_nodrain.go for why that exists.
func drainOnFullBatch() bool { return true }
