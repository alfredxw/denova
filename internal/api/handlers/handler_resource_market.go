package handlers

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
)

func (h *Handlers) HandleResourceMarketCatalog(ctx context.Context, c *app.RequestContext) {
	h.resourceMarketCatalog(ctx, c, false)
}

func (h *Handlers) HandleResourceMarketRefresh(ctx context.Context, c *app.RequestContext) {
	h.resourceMarketCatalog(ctx, c, true)
}

func (h *Handlers) resourceMarketCatalog(ctx context.Context, c *app.RequestContext, refresh bool) {
	result, err := h.app.ResourceMarket().Catalog(ctx, refresh)
	if err != nil {
		writeErrorKey(c, 503, "market.errors.catalogUnavailable")
		return
	}
	writeJSON(c, 200, result)
}
