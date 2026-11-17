// Package i18n selects the assistant's display language.
//
// The frontend detects the system language (and lets users override it in Settings),
// then updates the Go layer. Strings are kept beside their call sites with T.
// Device-side scripts receive the same selection through the UI_LANG environment variable.
package i18n

import "sync/atomic"

var english atomic.Bool

// Set selects English for "en" and Chinese otherwise.
func Set(lang string) { english.Store(lang == "en") }

// Lang returns the language code for device-side scripts.
func Lang() string {
	if english.Load() {
		return "en"
	}
	return "zh"
}

// T returns the string for the selected language.
func T(zh, en string) string {
	if english.Load() {
		return en
	}
	return zh
}
