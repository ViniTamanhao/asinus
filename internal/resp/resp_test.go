package resp

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestParserReadCommand(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:  "SET",
			input: "*3\r\n$3\r\nSET\r\n$5\r\nmykey\r\n$5\r\nhello\r\n",
			want:  []string{"SET", "mykey", "hello"},
		},
		{
			name:  "GET",
			input: "*2\r\n$3\r\nGET\r\n$5\r\nmykey\r\n",
			want:  []string{"GET", "mykey"},
		},
		{
			name:  "empty array",
			input: "*0\r\n",
			want:  nil,
		},
		{
			name:    "malformed header (no star)",
			input:   "3\r\n$3\r\nSET\r\n",
			wantErr: true,
		},
		{
			name:    "negative count",
			input:   "*-1\r\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser(strings.NewReader(tt.input))
			got, err := p.ReadCommand()
			if (err != nil) != tt.wantErr {
				t.Fatalf("ReadCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Fatalf("len = %d, want %d", len(got), len(tt.want))
				}
				for i := range got {
					if got[i] != tt.want[i] {
						t.Errorf("elem[%d] = %q, want %q", i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

func TestParserReadCommand_Truncated(t *testing.T) {
	p := NewParser(strings.NewReader("*1\r\n$3\r\nab"))
	_, err := p.ReadCommand()
	if err == nil {
		t.Fatal("expected error for truncated bulk string")
	}
}

func TestWriterSimpleString(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.WriteSimpleString("OK"); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "+OK\r\n" {
		t.Fatalf("got %q, want %q", got, "+OK\r\n")
	}
}

func TestWriterError(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.WriteError(errors.New("ERR something")); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "-ERR something\r\n" {
		t.Fatalf("got %q, want %q", got, "-ERR something\r\n")
	}
}

func TestWriterBulk(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"non-nil", []byte("hello"), "$5\r\nhello\r\n"},
		{"empty", []byte(""), "$0\r\n\r\n"},
		{"nil", nil, "$-1\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := NewWriter(&buf)
			if err := w.WriteBulk(tt.in); err != nil {
				t.Fatal(err)
			}
			if got := buf.String(); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriterInt(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.WriteInt(42); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != ":42\r\n" {
		t.Fatalf("got %q, want %q", got, ":42\r\n")
	}
}

