// This file only fences web/ (node_modules ships stray .go files) out of the root Go module,
// so `go ./...` tooling skips it. There is no Go code here.
module github.com/pocikode/ratatoskr/web

go 1.27
