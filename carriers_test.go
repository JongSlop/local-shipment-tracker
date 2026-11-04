package main

import "testing"

func TestDetect(t *testing.T) {
	for n, want := range map[string]string{"00340434161094042557": "dhl", "JJD000390012345": "dhl", "01270000000000": "auto", "H1234567890123456789": "auto"} {
		if got := detect(n); got != want {
			t.Errorf("%s: got %s want %s", n, got, want)
		}
	}
}

func TestParse17(t *testing.T) {
	r, err := parse17([]byte(`{"track_info":{"latest_status":{"status":"OutForDelivery"},"latest_event":{"description":"On vehicle"},
	"time_metrics":{"estimated_delivery_date":{"to":"2026-11-05T18:00:00+01:00"}},
	"tracking":{"providers":[{"events":[{"time_iso":"2026-11-04T08:00:00+01:00","description":"x","location":"Köln"}]}]}}}`))
	if err != nil || r.Status != stOut || r.ETA.IsZero() || len(r.Events) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestParseDHLUnknown(t *testing.T) {
	r, err := parseDHL([]byte(`{"sendungen":[{"sendungsdetails":{"sendungsverlauf":{"fortschritt":0,"events":[]}}}]}`))
	if err != nil || r.Status != stPending {
		t.Fatalf("%+v %v", r, err)
	}
}
