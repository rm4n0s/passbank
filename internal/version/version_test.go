package version

import (
	"runtime/debug"
	"testing"
)

func TestDescribe(t *testing.T) {
	tests := []struct {
		name string
		info debug.BuildInfo
		want string
	}{
		{"release", debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}, "v0.1.0"},
		{"no version", debug.BuildInfo{}, "(devel)"},
		{"checkout", debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "1a2b3c4d5e6f7a8b9c0d"},
		}}, "(devel) 1a2b3c4d5e6f"},
		{"modified checkout", debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc"}, {Key: "vcs.modified", Value: "true"},
		}}, "(devel) abc (modified)"},
		{"clean checkout", debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc"}, {Key: "vcs.modified", Value: "false"},
		}}, "(devel) abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describe(&tt.info); got != tt.want {
				t.Errorf("describe = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestString(t *testing.T) {
	if String() == "" {
		t.Error("String must not be empty")
	}
}
