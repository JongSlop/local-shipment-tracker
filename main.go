package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed index.html static
var assets embed.FS

type Shipment struct {
	ID          int64
	Number      string
	Carrier     string
	Label       string
	Status      string
	Text        string
	ETA         time.Time
	Events      []Event
	Registered  bool
	LastChecked time.Time
	Err         string
	L           string
}

var (
	db   *sql.DB
	key  = os.Getenv("TRACK17_KEY")
	tmpl = template.Must(template.New("index.html").Funcs(template.FuncMap{
		"day":    dayStr,
		"time":   func(t time.Time) string { return t.Format("02.01. 15:04") },
		"days":   daysUntil,
		"t":      tr,
		"list":   func(s ...string) []string { return s },
		"logo":   func(n string) template.HTML { return svg("logos", n) },
		"icon":   func(n string) template.HTML { return svg("icons", n) },
		"sticon": func(s string) template.HTML { return svg("icons", statusIcon[s]) },
	}).ParseFS(assets, "index.html"))
)

func daysUntil(t time.Time) int {
	n := time.Now()
	a := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	b := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.Local)
	return int(a.Sub(b).Hours() / 24)
}

func main() {
	var err error
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = "tracker.db"
	}
	if db, err = sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)"); err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS shipments (
		id INTEGER PRIMARY KEY, number TEXT UNIQUE NOT NULL, carrier TEXT NOT NULL, label TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending', text TEXT NOT NULL DEFAULT '', eta INTEGER NOT NULL DEFAULT 0,
		events TEXT NOT NULL DEFAULT '[]', registered INTEGER NOT NULL DEFAULT 0,
		last_checked INTEGER NOT NULL DEFAULT 0, err TEXT NOT NULL DEFAULT '', created INTEGER NOT NULL)`)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			refreshAll()
			time.Sleep(30 * time.Minute)
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.FileServerFS(assets))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { render(w, r, "index.html") })
	mux.HandleFunc("GET /list", func(w http.ResponseWriter, r *http.Request) { render(w, r, "list") })
	mux.HandleFunc("POST /lang", setLang)
	mux.HandleFunc("POST /add", add)
	mux.HandleFunc("POST /refresh/{id}", func(w http.ResponseWriter, r *http.Request) {
		if s, ok := byID(r.PathValue("id")); ok {
			refresh(s)
		}
		render(w, r, "list")
	})
	mux.HandleFunc("DELETE /shipments/{id}", func(w http.ResponseWriter, r *http.Request) {
		db.Exec("DELETE FROM shipments WHERE id=?", r.PathValue("id"))
		render(w, r, "list")
	})

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Println("listening on", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func add(w http.ResponseWriter, r *http.Request) {
	number := strings.ToUpper(strings.Join(strings.Fields(r.FormValue("number")), ""))
	carrier := r.FormValue("carrier")
	if len(number) < 5 || len(number) > 50 {
		http.Error(w, "invalid tracking number", 400)
		return
	}
	if carrier != "auto" && !carriers[carrier] {
		carrier = "auto"
	}
	if carrier == "auto" {
		carrier = detect(number)
	}
	res, err := db.Exec("INSERT OR IGNORE INTO shipments (number, carrier, label, created) VALUES (?,?,?,?)",
		number, carrier, strings.TrimSpace(r.FormValue("label")), time.Now().Unix())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if id, _ := res.LastInsertId(); id > 0 {
		if s, ok := byID(strconv.FormatInt(id, 10)); ok {
			refresh(s)
		}
	}
	render(w, r, "list")
}

const cols = "id, number, carrier, label, status, text, eta, events, registered, last_checked, err"

func scan(rows interface{ Scan(...any) error }) (s Shipment, err error) {
	var eta, lc int64
	var ev string
	if err = rows.Scan(&s.ID, &s.Number, &s.Carrier, &s.Label, &s.Status, &s.Text, &eta, &ev, &s.Registered, &lc, &s.Err); err != nil {
		return
	}
	if eta > 0 {
		s.ETA = time.Unix(eta, 0)
	}
	s.LastChecked = time.Unix(lc, 0)
	json.Unmarshal([]byte(ev), &s.Events)
	return
}

func byID(id string) (Shipment, bool) {
	s, err := scan(db.QueryRow("SELECT "+cols+" FROM shipments WHERE id=?", id))
	return s, err == nil
}

func all() (out []Shipment) {
	rows, err := db.Query("SELECT " + cols + " FROM shipments")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		if s, err := scan(rows); err == nil {
			out = append(out, s)
		}
	}
	return
}

func refreshAll() {
	for _, s := range all() {
		if s.Status != stDelivered {
			refresh(s)
			time.Sleep(time.Second) // 17track limit is 3 req/s
		}
	}
}

func refresh(s Shipment) {
	res, err := fetch(s, key)
	if err != nil {
		log.Println(s.Number, err)
		db.Exec("UPDATE shipments SET err=?, last_checked=? WHERE id=?", err.Error(), time.Now().Unix(), s.ID)
		return
	}
	sort.Slice(res.Events, func(i, j int) bool { return res.Events[i].Time.After(res.Events[j].Time) })
	ev, _ := json.Marshal(res.Events)
	db.Exec("UPDATE shipments SET status=?, text=?, eta=?, events=?, registered=?, last_checked=?, err='' WHERE id=?",
		res.Status, res.Text, etaUnix(res.ETA), ev, s.Registered || res.Registered, time.Now().Unix(), s.ID)
}

func etaUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

type page struct {
	Lang                string
	Soon, Transit, Done []Shipment
}

// Soon: out for delivery, at pickup point, or ETA within 2 days.
func render(w http.ResponseWriter, r *http.Request, name string) {
	p := page{Lang: lang(r)}
	for _, s := range all() {
		s.L = p.Lang
		switch {
		case s.Status == stDelivered:
			p.Done = append(p.Done, s)
		case s.Status == stOut || s.Status == stPickup || (!s.ETA.IsZero() && daysUntil(s.ETA) <= 2):
			p.Soon = append(p.Soon, s)
		default:
			p.Transit = append(p.Transit, s)
		}
	}
	byETA := func(l []Shipment) {
		sort.Slice(l, func(i, j int) bool {
			a, b := l[i].ETA, l[j].ETA
			if a.IsZero() != b.IsZero() {
				return !a.IsZero()
			}
			return a.Before(b)
		})
	}
	byETA(p.Soon)
	byETA(p.Transit)
	sort.Slice(p.Done, func(i, j int) bool { return p.Done[i].LastChecked.After(p.Done[j].LastChecked) })
	if err := tmpl.ExecuteTemplate(w, name, p); err != nil {
		log.Println(err)
	}
}
