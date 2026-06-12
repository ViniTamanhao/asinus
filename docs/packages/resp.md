# Package: `internal/resp`

Implements the Redis Serialization Protocol version 2 (RESP2) — both parsing and serialization.

## Overview

RESP2 is a line-oriented, type-prefixed text protocol. Every value begins with a single-character type indicator followed by a `\r\n` terminator. Client commands are always sent as **Arrays of Bulk Strings**; server replies can be Simple Strings, Errors, Integers, Bulk Strings, or Arrays.

This package provides:
- `Parser` — reads RESP-encoded commands from an `io.Reader`.
- `Writer` — writes RESP-encoded responses to an `io.Writer`.
- `EncodeCommand` — convenience function that returns the RESP wire bytes for a command slice.

## RESP2 Type Reference

| Prefix | Type | Example |
|---|---|---|
| `+` | Simple String | `+OK\r\n` |
| `-` | Error | `-ERR unknown command\r\n` |
| `:` | Integer | `:1\r\n` |
| `$` | Bulk String | `$5\r\nhello\r\n` |
| `*` | Array | `*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n` |

## `Parser`

```go
type Parser struct { r *bufio.Reader }

func NewParser(r io.Reader) *Parser
func (p *Parser) ReadCommand() ([]string, error)
```

`NewParser` wraps `r` in a 64 KiB `bufio.Reader`.

`ReadCommand` reads one complete client command:
1. Reads the Array header (`*<n>\r\n`) and parses `n`.
2. For each of the `n` elements, reads a Bulk String header (`$<len>\r\n`) then exactly `len` bytes plus a `\r\n` trailer.
3. Returns the command as `[]string`.

Special cases:
- `*0\r\n` — returns `nil, nil` (empty array).
- `$-1\r\n` — Null Bulk String; the corresponding element is an empty string `""`.
- On `io.EOF` at the array header, returns `nil, io.EOF` so callers can distinguish a clean disconnect from a parse error.

## `Writer`

```go
type Writer struct { w io.Writer }

func NewWriter(w io.Writer) *Writer
func (w *Writer) WriteSimpleString(s string) error
func (w *Writer) WriteError(errVal error) error
func (w *Writer) WriteBulk(b []byte) error
func (w *Writer) WriteInt(n int) error
func (w *Writer) WriteArray(items []string) error
```

- `WriteBulk(nil)` writes the Null Bulk String (`$-1\r\n`), used to signal a missing key on `GET`.
- `WriteArray` writes an Array header followed by each element as a Bulk String.

The `Writer` does not buffer internally. Wrap the underlying `io.Writer` in a `bufio.Writer` before passing it to `NewWriter` to batch multiple responses into one syscall.

## `EncodeCommand`

```go
func EncodeCommand(args []string) []byte
```

Returns the RESP wire bytes for `args` as an Array of Bulk Strings. Used by the AOF to serialize commands before appending them to disk, and by tests to build synthetic payloads.
