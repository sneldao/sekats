// Package quality computes the display-ready numbers for the Quality
// Surface screen from the raw fixture stats.
package quality

import (
	"github.com/sneldao/sekats/internal/store"
)

// Surface is everything the "/" screen needs, pre-computed so the template
// stays dumb.
type Surface struct {
	PolicySuggestionsTotal    int
	PolicySuggestionsPromoted int
	PromotionRatePct          float64

	LLMReviewsTotal int
	LinkedReviews   int
	ErrorBuckets    []store.ErrorBucket
	CleanReviews    int
	ErrorReviews    int
	ErrorRatePct    float64

	AgreementAgree   int
	AgreementTotal   int
	AgreementRatePct float64
}

// SourceInfo tells the screen where its numbers came from and how fresh
// they are: "live" (a read-only bridge over a synced snapshot of the
// agent's SQLite log) or "fixture" (the frozen export in data/).
type SourceInfo struct {
	Kind             string // "live" or "fixture"
	Freshness        string // human line, e.g. "synced 2026-08-26 18:39 UTC"
	LatestDecisionID int64
	LatestDecisionAt string // "" when the source carries no timestamps
}

// Build derives a Surface from the raw QualityStats fixture.
func Build(s store.QualityStats) Surface {
	surf := Surface{
		PolicySuggestionsTotal:    s.PolicySuggestionsTotal,
		PolicySuggestionsPromoted: s.PolicySuggestionsPromoted,
		LLMReviewsTotal:           s.LLMReviewsTotal,
		LinkedReviews:             s.LinkedReviews,
		ErrorBuckets:              s.LLMReviewsByError,
		AgreementAgree:            s.AgreementRate.Agree,
		AgreementTotal:            s.AgreementRate.Total,
	}

	if surf.PolicySuggestionsTotal > 0 {
		surf.PromotionRatePct = pct(surf.PolicySuggestionsPromoted, surf.PolicySuggestionsTotal)
	}

	for _, b := range s.LLMReviewsByError {
		if b.Bucket == "clean" {
			surf.CleanReviews += b.Count
		} else {
			surf.ErrorReviews += b.Count
		}
	}
	if total := surf.CleanReviews + surf.ErrorReviews; total > 0 {
		surf.ErrorRatePct = pct(surf.ErrorReviews, total)
	}

	if surf.AgreementTotal > 0 {
		surf.AgreementRatePct = pct(surf.AgreementAgree, surf.AgreementTotal)
	}

	return surf
}

func pct(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator) * 100
}
