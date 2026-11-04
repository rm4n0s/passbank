package secret

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// value is a distinctive secret, so that finding any part of it in an output is
// a leak.
const value = "hunter2-ZXCV-not-a-real-password"

func TestWipe(t *testing.T) {
	raw := []byte(value)
	s := New(raw)
	copyOfS := s
	s.Wipe()
	if !bytes.Equal(raw, make([]byte, len(raw))) {
		t.Errorf("the buffer is not zeroed after Wipe: %q", raw)
	}
	if s.Len() != 0 || copyOfS.Len() != 0 {
		t.Errorf("Len after Wipe = %d, copy %d; want 0 for both", s.Len(), copyOfS.Len())
	}
	s.Wipe() // wiping twice, and wiping the zero value, must not panic
	Bytes{}.Wipe()
}

func TestCloneIsIndependent(t *testing.T) {
	s := New([]byte(value))
	c := s.Clone()
	s.Wipe()
	if string(c.Reveal()) != value {
		t.Errorf("the clone changed when the original was wiped: %q", c.Reveal())
	}
	c.Wipe()
	if (Bytes{}).Clone().Len() != 0 {
		t.Error("the clone of the zero value must be empty")
	}
}

func TestEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b Bytes
		want bool
	}{
		{"same", New([]byte("abc")), New([]byte("abc")), true},
		{"different", New([]byte("abc")), New([]byte("abd")), false},
		{"prefix", New([]byte("abc")), New([]byte("ab")), false},
		{"empty and zero value", New(nil), Bytes{}, true},
		{"zero and zero", Bytes{}, Bytes{}, true},
		{"empty and not empty", Bytes{}, New([]byte("a")), false},
	}
	for _, tt := range tests {
		if got := tt.a.Equal(tt.b); got != tt.want {
			t.Errorf("%s: Equal = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestZeroValueAndNew(t *testing.T) {
	var zero Bytes
	if zero.Len() != 0 || zero.Reveal() != nil {
		t.Errorf("zero value: Len %d, Reveal %v", zero.Len(), zero.Reveal())
	}
	s := New([]byte(value))
	if s.Len() != len(value) || string(s.Reveal()) != value {
		t.Errorf("New: Len %d, Reveal %q", s.Len(), s.Reveal())
	}
}

// holder keeps a secret in unexported and exported fields, which is where fmt
// reaches into a value instead of calling its methods.
type holder struct {
	Public  Bytes
	private Bytes
}

func TestNeverPrinted(t *testing.T) {
	s := New([]byte(value))
	h := holder{Public: s, private: s}
	items := map[string]any{
		"value":   s,
		"pointer": &s,
		"struct":  h,
		"slice":   []Bytes{s},
		"map":     map[string]Bytes{"k": s},
	}
	verbs := []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%T", "%p"}
	// Any 4 bytes of the secret, in any of the ways fmt prints bytes.
	decimal := strings.Trim(fmt.Sprintf("%d", []byte(value[:4])), "[]")
	leaks := []string{value[:8], decimal, fmt.Sprintf("%x", value[:4]), fmt.Sprintf("%X", value[:4])}
	for name, item := range items {
		for _, verb := range verbs {
			out := fmt.Sprintf(verb, item)
			for _, leak := range leaks {
				if strings.Contains(out, leak) {
					t.Errorf("%s with %s leaks %q: %s", name, verb, leak, out)
				}
			}
		}
	}
	if got := fmt.Sprint(s); got != "[secret]" {
		t.Errorf("Sprint = %q, want [secret]", got)
	}
	if got := fmt.Sprintf("%#v", s); got != "[secret]" {
		t.Errorf("%%#v = %q, want [secret]", got)
	}
	if got := s.String(); got != "[secret]" {
		t.Errorf("String = %q, want [secret]", got)
	}
}

func TestNeverLogged(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	s := New([]byte(value))
	logger.Info("unlocked", "password", s, "group", slog.GroupValue(slog.Any("inner", s)))
	jsonBuf := &bytes.Buffer{}
	slog.New(slog.NewJSONHandler(jsonBuf, nil)).Info("unlocked", "password", s)
	for _, out := range []string{buf.String(), jsonBuf.String()} {
		if strings.Contains(out, value[:8]) {
			t.Errorf("a log line leaks the secret: %s", out)
		}
		if !strings.Contains(out, "[secret]") {
			t.Errorf("a log line does not show [secret]: %s", out)
		}
	}
}

func TestNeverMarshalled(t *testing.T) {
	s := New([]byte(value))
	out, err := json.Marshal(map[string]any{"password": s, "holder": holder{Public: s, private: s}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(out), value[:8]) {
		t.Errorf("JSON leaks the secret: %s", out)
	}
	if !strings.Contains(string(out), "[secret]") {
		t.Errorf("JSON does not show [secret]: %s", out)
	}
}
