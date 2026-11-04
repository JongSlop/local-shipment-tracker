package main

import (
	"html/template"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var msgs = map[string]map[string]string{
	"en": {
		"title": "Shipments", "number": "Tracking number", "label": "Label (optional)", "add": "Add", "auto": "Auto",
		"soon": "Arriving soon", "transit": "On the way", "done": "Delivered", "empty": "No shipments yet.",
		"today": "Today", "tomorrow": "Tomorrow", "checked": "checked", "history": "History", "confirm": "Delete?",
		"refresh": "Refresh", "delete": "Delete",
		"st_pending": "Pending", "st_transit": "In transit", "st_out_for_delivery": "Out for delivery",
		"st_pickup": "Ready for pickup", "st_delivered": "Delivered", "st_exception": "Problem",
	},
	"de": {
		"title": "Sendungen", "number": "Sendungsnummer", "label": "Bezeichnung (optional)", "add": "Hinzufügen", "auto": "Auto",
		"soon": "Kommt bald", "transit": "Unterwegs", "done": "Zugestellt", "empty": "Noch keine Sendungen.",
		"today": "Heute", "tomorrow": "Morgen", "checked": "geprüft", "history": "Verlauf", "confirm": "Löschen?",
		"refresh": "Aktualisieren", "delete": "Löschen",
		"st_pending": "Ausstehend", "st_transit": "Unterwegs", "st_out_for_delivery": "In Zustellung",
		"st_pickup": "Abholbereit", "st_delivered": "Zugestellt", "st_exception": "Problem",
	},
}

var weekdays = map[string][]string{
	"en": {"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
	"de": {"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"},
}

var statusIcon = map[string]string{
	stPending: "clock", stTransit: "truck", stOut: "truck",
	stPickup: "store", stDelivered: "circle-check", stException: "triangle-alert",
}

var carriers = map[string]bool{"dhl": true, "dpd": true, "gls": true, "hermes": true}

var svgComment = regexp.MustCompile(`^<!--.*?-->\s*`)

func lang(r *http.Request) string {
	if c, err := r.Cookie("lang"); err == nil && msgs[c.Value] != nil {
		return c.Value
	}
	if strings.HasPrefix(r.Header.Get("Accept-Language"), "en") {
		return "en"
	}
	return "de"
}

func tr(l, k string) string {
	if v, ok := msgs[l][k]; ok {
		return v
	}
	return k
}

func dayStr(l string, t time.Time) string {
	return weekdays[l][t.Weekday()] + t.Format(" 02.01.")
}

func svg(dir, name string) template.HTML {
	b, err := assets.ReadFile("static/" + dir + "/" + name + ".svg")
	if err != nil {
		return ""
	}
	return template.HTML(svgComment.ReplaceAll(b, nil)) // #nosec: embedded files only
}

func setLang(w http.ResponseWriter, r *http.Request) {
	if l := r.FormValue("l"); msgs[l] != nil {
		http.SetCookie(w, &http.Cookie{Name: "lang", Value: l, Path: "/", MaxAge: 365 * 24 * 3600, SameSite: http.SameSiteLaxMode})
	}
	w.Header().Set("HX-Refresh", "true")
}
