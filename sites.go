package serpkite

import (
	"cmp"
	"context"
	"net/http"
	"time"
)

// Map returns the URLs of a site: from robots.txt sitemaps (XML, gzip,
// RSS/Atom, plain text; sitemap indexes followed) and the start page's
// links, cleaned (fragments and tracking parameters dropped), kept on the
// site, filtered by IncludePaths / ExcludePaths and de-duplicated. Search
// ranks them by relevance and drops the rest. 1 credit; free when nothing
// is found.
func (c *Client) Map(ctx context.Context, p MapParams, opts ...RequestOption) (*MapResponse, error) {
	return post[MapResponse](ctx, c, "/v1/map", p, opts)
}

// Extract reads up to 20 URLs (HTML or PDF) as Markdown, text or HTML in
// one call, with optional query-ranked highlights (BM25), links and
// images. Each URL that comes back costs 1 credit (0.5 from cache); URLs
// that fail are listed in [ExtractResponse].Failed and cost nothing.
//
// Extract is billed per page even when the response is lost, so it is
// retried only on 429, and its per-attempt timeout covers the server's
// deadline (Timeout, default 50 s) plus 15 s.
func (c *Client) Extract(ctx context.Context, p ExtractParams, opts ...RequestOption) (*ExtractResponse, error) {
	var out ExtractResponse
	wait := time.Duration(cmp.Or(p.Timeout, extractDefaultTimeout)+15) * time.Second
	if _, err := c.decode(ctx, call{method: http.MethodPost, path: "/v1/extract", body: p, opts: opts, out: &out, rateLimitOnly: true, minTimeout: wait}); err != nil {
		return nil, err
	}
	return &out, nil
}

// extractDefaultTimeout is the server's extract deadline (seconds) when
// Timeout is 0.
const extractDefaultTimeout = 50
