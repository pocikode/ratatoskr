// Package rawhttp implements a lenient, byte-preserving HTTP/1.x reader and
// writer for the proxy, Replay and Automate paths (ADR-0003).
//
// Reading never rewrites input: [Message.Raw] returns exactly the bytes that
// came off the wire, header order, casing and spacing included. The only bytes
// it drops are blank lines before the start line, which RFC 9112 §2.2 lets a
// parser ignore. The structured view (start line, header fields, body framing)
// is parsed once, and anything odd about the input is recorded in
// [Message.Warnings] rather than silently fixed (docs/01-feature-spec.md
// F-NET-3).
//
// Bodies stream from the connection through [Message.Body]: they are never
// buffered whole, and only the first Limits.MaxBodyBytes are kept for [Raw].
// Untouched parts of a message keep their original bytes when a [Builder]
// re-serializes it.
package rawhttp
