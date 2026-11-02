// Package service is what background services share: goroutines that survive a panic,
// where each reports how it is doing, so the Dashboard can show their health without
// reaching into them, and the transport their requests to a venue go through.
package service

import (
	"runtime/debug"
	"vergex/internal/logger"
)

// Go launches fn in a new goroutine with automatic panic recovery. A panic in a bare
// goroutine tears down the entire process, so long-running or critical paths use this
// instead of `go func()`. If fn panics, the panic is logged under the given name (with
// stack trace) and the process continues running. An optional onPanic callback receives
// the recovered value.
func Go(name string, fn func(), onPanic ...func(recovered interface{})) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())
				logger.Errorf("🔥 [%s] goroutine panic recovered: %v\n%s", name, r, stack)

				for _, cb := range onPanic {
					func() {
						defer func() {
							if r2 := recover(); r2 != nil {
								logger.Errorf("🔥 onPanic callback itself panicked: %v", r2)
							}
						}()
						cb(r)
					}()
				}
			}
		}()
		fn()
	}()
}
