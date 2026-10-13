package main

import (
	"strings"
	"testing"
)

func TestScrubDiag(t *testing.T) {
	in := strings.Join([]string{
		`devicetoken=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abcDEF_123`,
		`password: hunter2`,
		`Authorization: Bearer abc.def`,
		`secret="s3 cr3t"`,
		`rm.network.authentication Fetching user token (fetchUserToken /src/x.cpp)`,
		`xochitl.service: Watchdog timeout`,
	}, "\n")
	out := scrubDiag(in)
	for _, leak := range []string{"eyJhbGci", "hunter2", "abc.def", "s3 cr3t"} {
		if strings.Contains(out, leak) {
			t.Errorf("没遮住 %q:\n%s", leak, out)
		}
	}
	// 普通日志原样保留, 不然诊断就没用了
	for _, keep := range []string{"Fetching user token (fetchUserToken", "Watchdog timeout"} {
		if !strings.Contains(out, keep) {
			t.Errorf("误伤了 %q:\n%s", keep, out)
		}
	}
}
