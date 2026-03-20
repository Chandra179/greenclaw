package browser

import (
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// blocked resource extensions and content types
var blockedExtensions = []string{
	".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".ico", ".bmp",
	".woff", ".woff2", ".ttf", ".eot", ".otf",
	".css",
}

var blockedDomains = []string{
	"googleads.g.doubleclick.net",
	"pagead2.googlesyndication.com",
	"ads.google.com",
	"adservice.google.com",
	"facebook.com/tr",
	"connect.facebook.net",
	"analytics.google.com",
	"google-analytics.com",
	"googletagmanager.com",
}

// SetupIntercept configures request interception to block images, fonts, CSS, and ads.
func SetupIntercept(page *rod.Page) {
	router := page.HijackRequests()
	router.MustAdd("*", func(ctx *rod.Hijack) {
		url := ctx.Request.URL().String()
		lower := strings.ToLower(url)

		// Block by file extension
		for _, ext := range blockedExtensions {
			if strings.HasSuffix(lower, ext) || strings.Contains(lower, ext+"?") {
				ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
				return
			}
		}

		// Block known ad/tracker domains
		for _, domain := range blockedDomains {
			if strings.Contains(lower, domain) {
				ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
				return
			}
		}

		ctx.ContinueRequest(&proto.FetchContinueRequest{})
	})
	go router.Run()
}
