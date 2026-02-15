package wg

import (
	"errors"
	"strings"
	"testing"
)

func TestClassifyNetlinkError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantContains string
	}{
		{
			"nil error",
			nil,
			"",
		},
		{
			"operation not permitted",
			errors.New("operation not permitted"),
			"CAP_NET_ADMIN",
		},
		{
			"file exists",
			errors.New("file exists"),
			"interface already exists",
		},
		{
			"no such device",
			errors.New("no such device"),
			"wireguard kernel module not loaded",
		},
		{
			"address already in use",
			errors.New("address already in use"),
			"listen port already bound",
		},
		{
			"no buffer space available",
			errors.New("no buffer space available"),
			"too many network interfaces",
		},
		{
			"invalid argument",
			errors.New("invalid argument"),
			"invalid configuration",
		},
		{
			"no such file or directory",
			errors.New("no such file or directory"),
			"wireguard kernel module not loaded",
		},
		{
			"permission denied",
			errors.New("permission denied"),
			"permission denied",
		},
		{
			"device or resource busy",
			errors.New("device or resource busy"),
			"interface is busy",
		},
		{
			"unknown error",
			errors.New("something unexpected happened"),
			"unknown netlink error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyNetlinkError(tt.err)

			if tt.err == nil {
				if got != "" {
					t.Errorf("ClassifyNetlinkError(nil) = %q, want empty", got)
				}
				return
			}

			if tt.wantContains != "" && !strings.Contains(got, tt.wantContains) {
				t.Errorf("ClassifyNetlinkError(%q) = %q, want containing %q",
					tt.err, got, tt.wantContains)
			}
		})
	}
}

func TestClassifyNetlinkError_AllHintsActionable(t *testing.T) {
	errs := []error{
		errors.New("operation not permitted"),
		errors.New("file exists"),
		errors.New("no such device"),
		errors.New("address already in use"),
		errors.New("no buffer space available"),
		errors.New("invalid argument"),
		errors.New("no such file or directory"),
		errors.New("permission denied"),
		errors.New("device or resource busy"),
		errors.New("something unknown"),
	}

	for _, err := range errs {
		hint := ClassifyNetlinkError(err)
		if hint == "" {
			t.Errorf("hint for %q should not be empty", err)
		}
		// Every hint should contain an em dash separating the error from the suggestion
		if !strings.Contains(hint, "—") {
			t.Errorf("hint for %q should contain actionable suggestion (em dash): %q", err, hint)
		}
	}
}
