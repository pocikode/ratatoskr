package history

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/query/core"
	"github.com/pocikode/ratatoskr/internal/query/httpql"
)

// SitemapNode is a derived host, path or query-name set in the filtered history.
type SitemapNode struct {
	ID, Kind, Label, URL, Host, Query string
	ParentID                          *string
	Port, RequestCount                int
	TLS, HasChildren                  bool
}

// sitemapID describes identity independently of filters and directory/file kind.
type sitemapID struct {
	Host  string   `json:"h"`
	Port  int      `json:"p"`
	TLS   bool     `json:"t"`
	Path  []string `json:"s,omitzero"`
	Names []string `json:"q,omitzero"`
}

func (id sitemapID) encode() string {
	b, _ := json.Marshal(id) // plain values cannot fail
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeSitemapID(src string) error {
	raw, err := base64.RawURLEncoding.DecodeString(src)
	if err != nil {
		return apperr.New(apperr.BadInput, "malformed sitemap node id")
	}
	var id sitemapID
	if err := json.Unmarshal(raw, &id); err != nil || id.encode() != src || id.Host == "" || id.Port < 1 || id.Port > 65535 {
		return apperr.New(apperr.BadInput, "malformed sitemap node id")
	}
	for _, seg := range id.Path {
		if seg == "" || strings.Contains(seg, "/") {
			return apperr.New(apperr.BadInput, "malformed sitemap node id")
		}
	}
	if len(id.Names) > 0 && (len(id.Path) == 0 || !slices.IsSorted(id.Names) || len(slices.Compact(slices.Clone(id.Names))) != len(id.Names)) {
		return apperr.New(apperr.BadInput, "malformed sitemap node id")
	}
	for _, name := range id.Names {
		if strings.ContainsAny(name, "&=") {
			return apperr.New(apperr.BadInput, "malformed sitemap node id")
		}
	}
	return nil
}

type sitemapEntry struct {
	SitemapNode
	identity sitemapID
	children map[string]*sitemapEntry
}

func sitemapChild(children map[string]*sitemapEntry, id sitemapID, parent *string, label, url, kind string) *sitemapEntry {
	key := id.encode()
	n := children[key]
	if n == nil {
		n = &sitemapEntry{SitemapNode: SitemapNode{
			ID: key, ParentID: parent, Kind: kind, Label: label, URL: url,
			Host: id.Host, Port: id.Port, TLS: id.TLS,
		}, identity: id, children: make(map[string]*sitemapEntry)}
		children[key] = n
	}
	return n
}

// SitemapNodes derives children lazily; recursive returns descendants in preorder.
func (s *Service) SitemapNodes(ctx context.Context, parentID, query, scopeID *string, recursive bool) ([]SitemapNode, error) {
	if parentID != nil {
		if err := decodeSitemapID(*parentID); err != nil {
			return nil, err
		}
	}
	f, err := s.filter(ctx, query, scopeID)
	if err != nil {
		return nil, err
	}
	var bar string
	if query != nil {
		n, err := httpql.Parse(*query)
		if err != nil {
			return nil, queryErr(err)
		}
		if n != nil {
			bar = *query
		}
	}
	db, err := s.db()
	if err != nil {
		return nil, err
	}
	rows, err := db.Reader().QueryContext(ctx,
		`SELECT r.host, r.port, r.tls, r.path, r.query, count(*)`+fromJoin+` WHERE `+f.where()+
			` GROUP BY r.host, r.port, r.tls, r.path, r.query`, f.sql.Args...)
	if err != nil {
		return nil, fmt.Errorf("history: sitemap: %w", err)
	}
	defer rows.Close()

	roots := make(map[string]*sitemapEntry)
	var parent *sitemapEntry
	for rows.Next() {
		var id sitemapID
		var path, queryText string
		var count int
		if err := rows.Scan(&id.Host, &id.Port, &id.TLS, &path, &queryText, &count); err != nil {
			return nil, fmt.Errorf("history: sitemap: %w", err)
		}
		scheme := "http"
		if id.TLS {
			scheme = "https"
		}
		url := scheme + "://" + net.JoinHostPort(id.Host, strconv.Itoa(id.Port))
		n := sitemapChild(roots, id, nil, url, url, "HOST")
		n.RequestCount += count
		if parentID != nil && n.ID == *parentID {
			parent = n
		}
		if strings.HasPrefix(path, "/") {
			for seg := range strings.SplitSeq(path, "/") {
				if seg == "" {
					continue
				}
				id.Path = append(slices.Clone(id.Path), seg)
				url += "/" + seg
				if n.Kind == "FILE" {
					n.Kind = "DIRECTORY"
				}
				n = sitemapChild(n.children, id, new(n.ID), seg, url, "FILE")
				n.RequestCount += count
				if parentID != nil && n.ID == *parentID {
					parent = n
				}
			}
		}
		if len(id.Path) == 0 {
			continue
		}
		for piece := range strings.SplitSeq(queryText, "&") {
			if piece != "" {
				name, _, _ := strings.Cut(piece, "=")
				id.Names = append(id.Names, name)
			}
		}
		if len(id.Names) > 0 {
			slices.Sort(id.Names)
			id.Names = slices.Compact(id.Names)
			label := "?" + strings.Join(id.Names, "&")
			child := sitemapChild(n.children, id, new(n.ID), label, url+label, "QUERY")
			child.RequestCount += count
			if parentID != nil && child.ID == *parentID {
				parent = child
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("history: sitemap: %w", err)
	}
	children := roots
	if parentID != nil {
		if parent == nil {
			return []SitemapNode{}, nil
		}
		children = parent.children
	}
	out := []SitemapNode{}
	var walk func(map[string]*sitemapEntry) error
	walk = func(children map[string]*sitemapEntry) error {
		sorted := make([]*sitemapEntry, 0, len(children))
		for _, n := range children {
			sorted = append(sorted, n)
		}
		slices.SortFunc(sorted, func(a, b *sitemapEntry) int {
			if a.Kind == "HOST" {
				return cmp.Or(cmp.Compare(a.Host, b.Host), cmp.Compare(a.Port, b.Port), cmp.Compare(strconv.FormatBool(a.TLS), strconv.FormatBool(b.TLS)))
			}
			if (a.Kind == "QUERY") != (b.Kind == "QUERY") {
				if a.Kind == "QUERY" {
					return 1
				}
				return -1
			}
			return cmp.Compare(a.Label, b.Label)
		})
		for _, n := range sorted {
			if recursive && len(out) == 5000 {
				return apperr.New(apperr.BadInput, "subtree has more than 5000 nodes")
			}
			n.HasChildren = len(n.children) > 0
			q := n.identity.query()
			if bar != "" {
				q = "(" + q + ") and (" + bar + "\n)"
			}
			if len(q) > core.MaxQueryLen {
				return apperr.New(apperr.BadQuery, "query too long")
			}
			if _, err := httpql.Parse(q); err != nil {
				return queryErr(err)
			}
			n.Query = q
			out = append(out, n.SitemapNode)
			if recursive {
				if err := walk(n.children); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(children); err != nil {
		return nil, err
	}
	return out, nil
}

func sitemapLiteral(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func (id sitemapID) query() string {
	q := "req.tls.eq:" + strconv.FormatBool(id.TLS) + " and req.host.eq:" + sitemapLiteral(id.Host) + " and req.port.eq:" + strconv.Itoa(id.Port)
	if len(id.Path) == 0 {
		return q
	}
	segments := make([]string, len(id.Path))
	for i, seg := range id.Path {
		segments[i] = regexp.QuoteMeta(seg)
	}
	path := "^/+" + strings.Join(segments, "/+")
	if len(id.Names) == 0 {
		return q + " and req.path.regex:" + sitemapLiteral(path+"(/|$)")
	}
	q += " and req.path.regex:" + sitemapLiteral(path+"/*$")
	names := make([]string, len(id.Names))
	for i, name := range id.Names {
		escaped := regexp.QuoteMeta(name)
		names[i] = escaped + "(?:=[^&]*)?"
		required := "(^|&)" + escaped + "(=|&|$)"
		if name == "" {
			names[i] = "=[^&]*"
			required = "(^|&)="
		}
		q += " and req.query.regex:" + sitemapLiteral(required)
	}
	return q + " and req.query.regex:" + sitemapLiteral("^&*(?:(?:"+strings.Join(names, "|")+")(?:&+|$))+$")
}
