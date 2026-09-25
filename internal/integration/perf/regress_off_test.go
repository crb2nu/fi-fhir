//go:build integration && !perfregress

package perf

import "time"

// injectedRegression is zero in every build but the negative control; see
// regress_on_test.go.
const injectedRegression time.Duration = 0
