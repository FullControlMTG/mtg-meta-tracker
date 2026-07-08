package analytics

import "math"

const (
	wilsonZ = 1.96 // ~95% one-sided lower bound
	shrinkK = 10.0 // Bayesian pseudo-count, expressed in games
)

// wilsonLower is the Wilson score interval lower bound for wins successes out of
// games trials. Returns 0 when games == 0.
func wilsonLower(wins, games int, z float64) float64 {
	if games == 0 {
		return 0
	}
	n := float64(games)
	phat := float64(wins) / n
	z2 := z * z
	denom := 1 + z2/n
	center := phat + z2/(2*n)
	margin := z * math.Sqrt((phat*(1-phat)+z2/(4*n))/n)
	lo := (center - margin) / denom
	if lo < 0 {
		return 0
	}
	return lo
}

// shrink pulls an observed winrate toward the global mean mu with pseudo-count k
// (Beta-Binomial): (wins + k*mu) / (games + k).
func shrink(wins, games int, mu, k float64) float64 {
	return (float64(wins) + k*mu) / (float64(games) + k)
}

// ratePtr returns wins/games as a pointer, or nil when games == 0.
func ratePtr(wins, games int) *float64 {
	if games == 0 {
		return nil
	}
	r := float64(wins) / float64(games)
	return &r
}
