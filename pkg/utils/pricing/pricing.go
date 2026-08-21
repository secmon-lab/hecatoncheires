// Package pricing turns a model's token counts into money.
//
// It exists because a token is not a unit of cost: this application runs one
// agent on an expensive model and another on a cheap one, so a budget counted in
// tokens spends most of itself on whichever agent happens to be chatty rather
// than on whichever is expensive.
//
// It is domain-agnostic and holds no price table of its own: which models exist
// and what they cost is operator configuration (the global config's
// [[llm_model]] sections), not a fact this package can know.
package pricing

import (
	"math"
	"strconv"
)

// NanoUSD is an amount of money in 1e-9 USD.
//
// AN INTEGER, so no accounting ever passes through a float. The unit is nano
// rather than micro because a single input token of a cheap model costs a few
// hundred of these — at micro resolution that token would round to zero and a
// budget could never be reached. int64 holds about 9.2 billion USD, several
// orders of magnitude past any plausible total.
type NanoUSD int64

// USD formats the amount for a prompt or a message, e.g. "$3.21".
//
// Rounded to the nearest cent, which is the resolution a person reads a budget
// at. This is the ONLY place a money value is turned into a decimal, and it
// happens on the way out.
//
// THE MAGNITUDE IS TAKEN UNSIGNED, and the wrap is the mechanism rather than a
// hazard: negating an int64 in place cannot represent math.MinInt64, so it would
// leave the value negative and every digit after it wrong. Reinterpreting the
// bits and negating in uint64 gives |n| for every input, that one included.
func (n NanoUSD) USD() string {
	neg := n < 0
	magnitude := uint64(n) // #nosec G115 -- the two's-complement wrap is what makes MinInt64 work; see above
	if neg {
		magnitude = -magnitude
	}
	// +5e6 rounds to nearest rather than truncating, so 0.005 USD reads as
	// "$0.01" and not "$0.00".
	cents := (magnitude + 5_000_000) / 10_000_000
	out := "$" + strconv.FormatUint(cents/100, 10) + "." + pad2(cents%100)
	if neg {
		return "-" + out
	}
	return out
}

func pad2(v uint64) string {
	s := strconv.FormatUint(v, 10)
	if len(s) < 2 {
		return "0" + s
	}
	return s
}

// FromUSD converts a budget written in dollars into NanoUSD. It is the boundary
// conversion for a configuration value, and the only float the money path sees.
func FromUSD(usd float64) NanoUSD {
	return NanoUSD(math.Round(usd * 1e9))
}

// FromUSDPerMTok converts a published price — dollars per 1M tokens, the unit
// every provider's pricing page uses — into NanoUSD per single token.
//
// The unit is the operator's, deliberately: a per-token nano figure is a
// ten-digit integer nobody can check against a price page, and a misplaced zero
// there is a budget wrong by a factor of ten.
func FromUSDPerMTok(usd float64) NanoUSD {
	return NanoUSD(math.Round(usd * 1e3))
}

// Rate is one model's published price, in NanoUSD per token.
//
// Four components rather than one, because a cached token is not priced like a
// fresh one and a token WRITTEN to cache is priced above one. Collapsing them
// loses the discount in one direction and the premium in the other.
type Rate struct {
	// Input is a token that was neither served from cache nor written to it.
	Input NanoUSD
	// Output is a generated token.
	Output NanoUSD
	// CacheRead is a token served from the prompt cache. Zero for a model that
	// has no cache.
	CacheRead NanoUSD
	// CacheWrite is a token written to the prompt cache, normally at a premium
	// over Input. Zero for a provider that does not bill a per-token write.
	CacheWrite NanoUSD
}

// Cost prices one run, or one generate.
//
// input is the provider's TOTAL input count, of which cacheRead and cacheWrite
// are components — the relation agentkit.Metrics documents (InputTokens =
// uncached input + CacheCreationInputTokens + CacheReadInputTokens).
//
// The uncached remainder is CLAMPED AT ZERO rather than trusted. A provider that
// reported input exclusive of its cache components would drive it negative, and
// a negative charge would credit a budget for spending money. The clamp never
// fires for the providers reached today; it exists so that a future one cannot
// silently pay a run to execute.
func (r Rate) Cost(input, output, cacheRead, cacheWrite int64) NanoUSD {
	uncached := max(input-cacheRead-cacheWrite, 0)
	return NanoUSD(uncached)*r.Input +
		NanoUSD(cacheRead)*r.CacheRead +
		NanoUSD(cacheWrite)*r.CacheWrite +
		NanoUSD(output)*r.Output
}

// IsPriced reports whether the rate can bound a budget at all.
//
// A zero input or output price is the dangerous shape: it prices a model at
// nothing, which makes its budget infinite — the exact failure a money budget
// exists to prevent, arrived at from the other side. A zero cache price is
// legitimate (a provider that bills no per-token cache write, or none at all).
func (r Rate) IsPriced() bool { return r.Input > 0 && r.Output > 0 }
