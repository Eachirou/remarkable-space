//go:build !linux

package handwriting

import "time"

func preciseSleep(d time.Duration) { time.Sleep(d) }
