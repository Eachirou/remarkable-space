package space

import "testing"

// The default zone must be one the settings list can show as selected.
func TestDefaultTimezoneIsListed(t *testing.T) {
	for _, c := range tzChoices {
		if c.Name == defaultTimezone {
			return
		}
	}
	t.Errorf("defaultTimezone %q is not in tzChoices", defaultTimezone)
}

// With no preference, no $TZ and no config file, the shell must not show Beijing time.
func TestLocalTimezoneDefault(t *testing.T) {
	t.Setenv("TZ", "")
	t.Setenv("HOME", t.TempDir()) // no ~/.config/rmkit-cn/space.json
	h := &Handler{Reg: &Registry{DataDir: t.TempDir()}} // and no shell/config.json
	name, off := h.localTimezone()
	if name != defaultTimezone {
		t.Errorf("zone = %q, want %q", name, defaultTimezone)
	}
	if off != 60 && off != 120 { // CET or CEST
		t.Errorf("offset = %d minutes, want 60 (winter) or 120 (summer)", off)
	}
}
