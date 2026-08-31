package pricing_test

import (
	"math"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/pricing"
)

func TestNanoUSDFormat(t *testing.T) {
	testCases := map[string]struct {
		value pricing.NanoUSD
		want  string
	}{
		"zero":                      {value: 0, want: "$0.00"},
		"one dollar":                {value: 1_000_000_000, want: "$1.00"},
		"cents":                     {value: 3_210_000_000, want: "$3.21"},
		"rounds a half cent up":     {value: 5_000_000, want: "$0.01"},
		"rounds below a half cent":  {value: 4_999_999, want: "$0.00"},
		"negative":                  {value: -1_500_000_000, want: "-$1.50"},
		"large":                     {value: 987_650_000_000, want: "$987.65"},
		"single digit cents padded": {value: 1_050_000_000, want: "$1.05"},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			gt.String(t, tc.value.USD()).Equal(tc.want)
		})
	}
}

// TestNanoUSDFormatAtMinInt64 pins the reason the magnitude is taken in uint64:
// negating math.MinInt64 as an int64 leaves it negative, which would render
// every digit wrong instead of merely the sign.
func TestNanoUSDFormatAtMinInt64(t *testing.T) {
	got := pricing.NanoUSD(math.MinInt64).USD()
	gt.String(t, got).HasPrefix("-$")
	gt.String(t, got).Equal("-$9223372036.85")
}

// TestFloorCent pins why the method exists: an amount that is both SHOWN and
// ENFORCED must render as no more than it is. USD rounds to the nearest cent, so
// $0.856 reads as "$0.86" — and a reader allocating the $0.86 it was told it had
// would then be refused against the $0.856 actually available.
func TestFloorCent(t *testing.T) {
	testCases := map[string]struct {
		value    pricing.NanoUSD
		want     pricing.NanoUSD
		wantText string
	}{
		"a sub-cent remainder that USD would round up": {
			value: 856_000_000, want: 850_000_000, wantText: "$0.85",
		},
		"a sub-cent remainder that USD would round down": {
			value: 854_000_000, want: 850_000_000, wantText: "$0.85",
		},
		"a whole cent is unchanged": {
			value: 850_000_000, want: 850_000_000, wantText: "$0.85",
		},
		"zero": {value: 0, want: 0, wantText: "$0.00"},
		"less than a cent floors to nothing": {
			value: 4_000_000, want: 0, wantText: "$0.00",
		},
		// Away from zero, as the name says. Nothing here passes a negative — a
		// remaining allowance is clamped at zero — but rounding one the other way
		// would make the method a trap for a later caller.
		"a negative amount floors away from zero": {
			value: -856_000_000, want: -860_000_000, wantText: "-$0.86",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got := tc.value.FloorCent()
			gt.Value(t, got).Equal(tc.want)
			// The whole point: the floored amount renders exactly, so the figure a
			// reader is shown is the figure that is enforced.
			gt.String(t, got.USD()).Equal(tc.wantText)
		})
	}
}

// TestNanoUSDValue pins the conversion the GraphQL field carries. It is the one
// place a money value becomes a float, so what matters is that the stored integer
// survives the trip: a fraction of a cent must not round to zero, and an amount
// inside float64's exact integer range must come back exactly.
func TestNanoUSDValue(t *testing.T) {
	testCases := map[string]struct {
		nano pricing.NanoUSD
		want float64
	}{
		"zero":                     {nano: 0, want: 0},
		"one nano":                 {nano: 1, want: 1e-9},
		"a fraction of a cent":     {nano: 290_000, want: 0.00029},
		"a whole cent":             {nano: 10_000_000, want: 0.01},
		"a whole dollar":           {nano: 1_000_000_000, want: 1},
		"negative":                 {nano: -2_500_000_000, want: -2.5},
		"the exactness bound":      {nano: 1 << 53, want: float64(1<<53) / 1e9},
		"past the exactness bound": {nano: math.MaxInt64, want: float64(math.MaxInt64) / 1e9},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			gt.Value(t, tc.nano.USDValue()).Equal(tc.want)
		})
	}
}

// TestNanoUSDValueKeepsCentsExact walks every cent of a dollar, because the value
// a page renders is read at two decimals: a conversion that drifted by a fraction
// of a cent would display the wrong amount without failing any single case above.
func TestNanoUSDValueKeepsCentsExact(t *testing.T) {
	for cents := range 100 {
		nano := pricing.NanoUSD(cents) * 10_000_000
		gt.Number(t, math.Abs(nano.USDValue()-float64(cents)/100)).Less(1e-12)
	}
}

func TestFromUSD(t *testing.T) {
	testCases := map[string]struct {
		usd  float64
		want pricing.NanoUSD
	}{
		"whole dollars":  {usd: 2, want: 2_000_000_000},
		"cents":          {usd: 0.5, want: 500_000_000},
		"small fraction": {usd: 0.000000001, want: 1},
		"rounds":         {usd: 0.0000000004, want: 0},
		"zero":           {usd: 0, want: 0},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			gt.Value(t, pricing.FromUSD(tc.usd)).Equal(tc.want)
		})
	}
}

// TestFromUSDPerMTok covers the unit an operator actually writes: dollars per
// 1M tokens, taken from a provider's pricing page.
func TestFromUSDPerMTok(t *testing.T) {
	testCases := map[string]struct {
		usdPerMTok float64
		want       pricing.NanoUSD
	}{
		"five dollars per mtok":  {usdPerMTok: 5, want: 5000},
		"cheap flash input":      {usdPerMTok: 0.75, want: 750},
		"cache read discount":    {usdPerMTok: 0.075, want: 75},
		"cache write premium":    {usdPerMTok: 6.25, want: 6250},
		"rounds to nearest nano": {usdPerMTok: 0.0004, want: 0},
		"smallest priced value":  {usdPerMTok: 0.001, want: 1},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			gt.Value(t, pricing.FromUSDPerMTok(tc.usdPerMTok)).Equal(tc.want)
		})
	}
}

func TestRateCost(t *testing.T) {
	// $5 / $25 / $0.50 / $6.25 per MTok.
	rate := pricing.Rate{Input: 5000, Output: 25000, CacheRead: 500, CacheWrite: 6250}

	testCases := map[string]struct {
		input, output, cacheRead, cacheWrite int64
		want                                 pricing.NanoUSD
	}{
		"no cache": {
			input: 1000, output: 200,
			// 1000*5000 + 200*25000
			want: 5_000_000 + 5_000_000,
		},
		"cache read is charged at its discount, not at the input rate": {
			input: 1000, output: 0, cacheRead: 900,
			// 100 uncached at 5000 + 900 read at 500
			want: 500_000 + 450_000,
		},
		"cache write is charged at its premium": {
			input: 1000, output: 0, cacheWrite: 1000,
			want: 1000 * 6250,
		},
		"both cache components are subtracted from the total input": {
			input: 1000, output: 0, cacheRead: 400, cacheWrite: 400,
			// 200 uncached + 400 read + 400 write
			want: 200*5000 + 400*500 + 400*6250,
		},
		"zero everything": {want: 0},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got := rate.Cost(tc.input, tc.output, tc.cacheRead, tc.cacheWrite)
			gt.Value(t, got).Equal(tc.want)
		})
	}
}

// TestRateCostClampsTheUncachedRemainder pins the guard against a provider that
// reports input exclusive of its cache components: without the clamp the
// remainder goes negative and the charge credits the budget.
func TestRateCostClampsTheUncachedRemainder(t *testing.T) {
	rate := pricing.Rate{Input: 5000, Output: 25000, CacheRead: 500}

	got := rate.Cost(100, 0, 900, 0)

	gt.Value(t, got).Equal(pricing.NanoUSD(900 * 500))
	gt.Number(t, int64(got)).Greater(0)
}

func TestRateIsPriced(t *testing.T) {
	testCases := map[string]struct {
		rate pricing.Rate
		want bool
	}{
		"fully priced": {rate: pricing.Rate{Input: 1, Output: 1}, want: true},
		"cache prices are optional": {
			rate: pricing.Rate{Input: 1000, Output: 5000}, want: true,
		},
		"zero input makes the budget unbounded":  {rate: pricing.Rate{Output: 5000}},
		"zero output makes the budget unbounded": {rate: pricing.Rate{Input: 1000}},
		"empty":                                  {rate: pricing.Rate{}},
		"negative input":                         {rate: pricing.Rate{Input: -1, Output: 1}},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			gt.Value(t, tc.rate.IsPriced()).Equal(tc.want)
		})
	}
}
