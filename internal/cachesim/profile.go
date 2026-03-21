package cachesim

type SessionProfile struct {
	StableFraction      float64
	HistoryFraction     float64
	TailFraction        float64
	TailExpansionFactor float64
}

func ProfileFromTargetCostRatio(pct int) *SessionProfile {
	if pct <= 0 {
		return nil
	}
	if pct < 15 {
		pct = 15
	}
	if pct > 90 {
		pct = 90
	}

	progress := float64(pct-15) / float64(90-15)
	stable := lerp(0.62, 0.30, progress)
	history := lerp(0.30, 0.12, progress)
	tail := 1.0 - stable - history

	return &SessionProfile{
		StableFraction:      stable,
		HistoryFraction:     history,
		TailFraction:        tail,
		TailExpansionFactor: lerp(1.0, 1.6, progress),
	}
}

func lerp(start float64, end float64, progress float64) float64 {
	return start + (end-start)*progress
}
