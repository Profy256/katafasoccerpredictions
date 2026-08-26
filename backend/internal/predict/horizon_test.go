package predict_test

import (
	"testing"
	"time"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/predict"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/tips"
)

// Horizon and MaxWindowDays are set in different packages for different
// reasons, and the shortlist quietly stops working when they disagree.
//
// A fixture with no prediction is invisible to everything — the feed drops it,
// so it cannot be a candidate either. If the shortlist may reach three days
// forward on a starved matchday but the model only prices two, the third day
// is empty and the window caps itself with nothing to show for it. That was
// the live state: MaxWindowDays said 3, the horizon said 48 hours, and the
// site showed one fixture.
//
// Nothing enforces this at compile time, so it is asserted here.
func TestHorizonCoversShortlistWindow(t *testing.T) {
	window := time.Duration(tips.MaxWindowDays) * 24 * time.Hour
	if predict.Horizon < window {
		t.Fatalf("predict.Horizon is %v but the shortlist may reach %v (tips.MaxWindowDays=%d); "+
			"the window would select over days that have nothing priced on them",
			predict.Horizon, window, tips.MaxWindowDays)
	}
}

// The horizon is only useful up to what has actually been ingested: fixtures
// are synced 14 days out, so pricing further is pricing fixtures that are not
// there. This is a sanity bound, not a target — see the doc comment on Horizon
// for why it is not simply maximised.
func TestHorizonWithinFixtureSync(t *testing.T) {
	const fixtureSync = 14 * 24 * time.Hour
	if predict.Horizon > fixtureSync {
		t.Fatalf("predict.Horizon is %v but fixtures are only synced %v ahead",
			predict.Horizon, fixtureSync)
	}
}
