package markov

import "markov_screener/internal/market"

func ClassifyRegime(
	state market.State,
	continuation float64,
	reversal float64,
	persistence float64,
	continuation2 float64,
	reversal2 float64,
	persistence2 float64,
	entropy float64,
) string {
	improving := continuation2 > continuation && reversal2 <= reversal
	degrading := continuation2 < continuation && reversal2 > reversal

	switch state {
	case market.StrongUp, market.WeakUp:
		if continuation >= 0.30 && reversal <= 0.22 && improving && entropy <= 1.40 {
			return "trend_long"
		}
		if degrading || reversal >= 0.45 {
			return "transition"
		}
		return "up_unstable"

	case market.StrongDown, market.WeakDown:
		if continuation >= 0.30 && reversal <= 0.22 && improving && entropy <= 1.40 {
			return "trend_short"
		}
		if degrading || reversal >= 0.45 {
			return "transition"
		}
		return "down_unstable"

	case market.Flat:
		if persistence >= 0.50 && persistence2 >= 0.48 && entropy <= 1.35 {
			return "range_clean"
		}
		if degrading || entropy > 1.35 {
			return "range_noisy"
		}
		return "neutral"

	default:
		return "unknown"
	}
}
func HumanSignal(
	state market.State,
	regimeClass string,
	continuation float64,
	reversal float64,
	continuation2 float64,
	reversal2 float64,
	entropy float64,
) string {
	improvingLong := continuation2 >= continuation && reversal2 <= reversal
	improvingShort := continuation2 >= continuation && reversal2 <= reversal
	degrading := continuation2 < continuation && reversal2 > reversal

	switch regimeClass {
	case "trend_long":
		if (state == market.StrongUp || state == market.WeakUp) &&
			continuation2 >= 0.35 &&
			reversal2 <= 0.22 {
			return "strong long"
		}
		return "weak long"

	case "trend_short":
		if (state == market.StrongDown || state == market.WeakDown) &&
			continuation2 >= 0.35 &&
			reversal2 <= 0.22 {
			return "strong short"
		}
		return "weak short"

	case "up_unstable":
		if (state == market.StrongUp || state == market.WeakUp) &&
			continuation > reversal &&
			entropy <= 1.50 {

			if improvingLong && continuation2 >= continuation {
				return "strong long"
			}

			if continuation >= 0.30 {
				return "weak long"
			}
		}
		return "flat / wait"

	case "down_unstable":
		if (state == market.StrongDown || state == market.WeakDown) &&
			entropy <= 1.50 {

			if improvingShort && continuation2 >= continuation {
				return "strong short"
			}

			if continuation >= 0.30 {
				return "weak short"
			}
		}
		return "flat / wait"

	case "transition":
		if (state == market.StrongUp || state == market.WeakUp) &&
			continuation >= 0.50 &&
			reversal <= 0.25 &&
			entropy <= 1.52 {
			return "weak long"
		}
		if (state == market.StrongDown || state == market.WeakDown) &&
			continuation >= 0.50 &&
			reversal <= 0.25 &&
			entropy <= 1.52 {
			return "weak short"
		}
		if degrading {
			return "flat / wait"
		}
		return "flat / wait"

	case "range_clean":
		return "flat / wait"

	case "range_noisy":
		if entropy > 1.50 {
			return "avoid"
		}
		return "flat / wait"

	case "neutral":
		return "flat / wait"

	default:
		return "avoid"
	}
}
