package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

const (
	stPending   = "pending"
	stTransit   = "transit"
	stOut       = "out_for_delivery"
	stPickup    = "pickup"
	stDelivered = "delivered"
	stException = "exception"
)

type Event struct {
	Time     time.Time `json:"time"`
	Text     string    `json:"text"`
	Location string    `json:"location"`
}

type Result struct {
	Status     string
	Text       string
	ETA        time.Time
	Events     []Event
	Registered bool
}

var httpc = &http.Client{Timeout: 20 * time.Second}

// 17track carrier codes. Missing/0 = let 17track auto-detect.
var codes17 = map[string]int{"dpd": 100007, "gls": 101070, "hermes": 100031}

// Only DHL formats that are unambiguous. Everything else goes to 17track auto-detect.
var dhlRe = regexp.MustCompile(`^(\d{20}|JJD\d+|JVGL\d+|[A-Z]{2}\d{9}DE)$`)

func detect(number string) string {
	if dhlRe.MatchString(number) {
		return "dhl"
	}
	return "auto"
}

func fetch(s Shipment, key string) (Result, error) {
	if s.Carrier == "dhl" {
		return fetchDHL(s.Number)
	}
	if key == "" {
		return Result{}, fmt.Errorf("TRACK17_KEY not set")
	}
	return fetch17(s, key)
}

// ---- DHL (unofficial public endpoint, no key) ----

func fetchDHL(number string) (Result, error) {
	req, _ := http.NewRequest("GET", "https://www.dhl.de/int-verfolgen/data/search?language=de&piececode="+url.QueryEscape(number), nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0")
	req.Header.Set("Accept", "application/json")
	resp, err := httpc.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return Result{}, fmt.Errorf("dhl http %d", resp.StatusCode)
	}
	return parseDHL(b)
}

func parseDHL(b []byte) (Result, error) {
	var r struct {
		RateLimited bool `json:"rateLimited"`
		Sendungen   []struct {
			Details struct {
				Verlauf struct {
					Fortschritt     int    `json:"fortschritt"`
					AktuellerStatus string `json:"aktuellerStatus"`
					Events          []struct {
						Datum  string `json:"datum"`
						Status string `json:"status"`
						Ort    string `json:"ort"`
					} `json:"events"`
				} `json:"sendungsverlauf"`
				Zugestellt bool `json:"istZugestellt"`
				// ponytail: ETA field names unverified without a live parcel; check raw JSON and adjust.
				Zustellung struct {
					Von string `json:"zustellzeitfensterVon"`
					Bis string `json:"zustellzeitfensterBis"`
				} `json:"zustellung"`
			} `json:"sendungsdetails"`
		} `json:"sendungen"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return Result{}, err
	}
	if r.RateLimited {
		return Result{}, fmt.Errorf("dhl rate limited")
	}
	if len(r.Sendungen) == 0 {
		return Result{Status: stPending}, nil
	}
	d := r.Sendungen[0].Details
	res := Result{Status: stPending, Text: d.Verlauf.AktuellerStatus}
	for _, e := range d.Verlauf.Events {
		res.Events = append(res.Events, Event{parseTime(e.Datum), e.Status, e.Ort})
	}
	if len(res.Events) > 0 && res.Text == "" {
		res.Text = res.Events[0].Text
	}
	switch p := d.Verlauf.Fortschritt; {
	case d.Zugestellt || p >= 5:
		res.Status = stDelivered
	case p == 4:
		res.Status = stOut
	case p >= 2:
		res.Status = stTransit
	}
	res.ETA = parseTime(d.Zustellung.Bis)
	return res, nil
}

// ---- 17track (DPD, GLS, Hermes, Amazon, ...) ----

type resp17 struct {
	Data struct {
		Accepted []json.RawMessage `json:"accepted"`
		Rejected []struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"rejected"`
	} `json:"data"`
}

func post17(key, path string, body any) (out resp17, err error) {
	buf, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "https://api.17track.net/track/v2.4/"+path, bytes.NewReader(buf))
	req.Header.Set("17token", key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpc.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return out, fmt.Errorf("17track http %d", resp.StatusCode)
	}
	err = json.NewDecoder(resp.Body).Decode(&out)
	return
}

func fetch17(s Shipment, key string) (Result, error) {
	item := map[string]any{"number": s.Number}
	if c := codes17[s.Carrier]; c != 0 {
		item["carrier"] = c
	}
	if !s.Registered {
		out, err := post17(key, "register", []any{item})
		if err != nil {
			return Result{}, err
		}
		for _, rj := range out.Data.Rejected {
			if rj.Error.Code != -18019901 { // already registered is fine
				return Result{}, fmt.Errorf("17track: %s", rj.Error.Message)
			}
		}
		s.Registered = true
	}
	out, err := post17(key, "gettrackinfo", []any{item})
	if err != nil {
		return Result{}, err
	}
	for _, rj := range out.Data.Rejected {
		if rj.Error.Code == -18019909 { // no data yet
			return Result{Status: stPending, Registered: true}, nil
		}
		return Result{}, fmt.Errorf("17track: %s", rj.Error.Message)
	}
	if len(out.Data.Accepted) == 0 {
		return Result{Status: stPending, Registered: true}, nil
	}
	res, err := parse17(out.Data.Accepted[0])
	res.Registered = true
	return res, err
}

func parse17(b []byte) (Result, error) {
	var a struct {
		Info struct {
			Latest struct {
				Status string `json:"status"`
			} `json:"latest_status"`
			Event struct {
				Description string `json:"description"`
			} `json:"latest_event"`
			Metrics struct {
				ETA struct {
					To   string `json:"to"`
					From string `json:"from"`
				} `json:"estimated_delivery_date"`
			} `json:"time_metrics"`
			Tracking struct {
				Providers []struct {
					Events []struct {
						Time        string `json:"time_iso"`
						Description string `json:"description"`
						Location    string `json:"location"`
					} `json:"events"`
				} `json:"providers"`
			} `json:"tracking"`
		} `json:"track_info"`
	}
	if err := json.Unmarshal(b, &a); err != nil {
		return Result{}, err
	}
	res := Result{Status: map17[a.Info.Latest.Status], Text: a.Info.Event.Description}
	if res.Status == "" {
		res.Status = stPending
	}
	if t := parseTime(a.Info.Metrics.ETA.To); !t.IsZero() {
		res.ETA = t
	} else {
		res.ETA = parseTime(a.Info.Metrics.ETA.From)
	}
	for _, p := range a.Info.Tracking.Providers {
		for _, e := range p.Events {
			res.Events = append(res.Events, Event{parseTime(e.Time), e.Description, e.Location})
		}
	}
	return res, nil
}

var map17 = map[string]string{
	"InfoReceived": stPending, "NotFound": stPending, "InTransit": stTransit,
	"OutForDelivery": stOut, "AvailableForPickup": stPickup, "Delivered": stDelivered,
	"DeliveryFailure": stException, "Exception": stException, "Expired": stException,
}

func parseTime(s string) time.Time {
	for _, l := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}
