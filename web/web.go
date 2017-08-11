package web

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ericdaugherty/feedmonitor/db"
	"github.com/sirupsen/logrus"
)

var log *logrus.Entry
var server *http.Server

var tmpl *template.Template
var templates map[string]*template.Template

// StartWebserver initializes the webserver and starts the listener.
func StartWebserver(ctx context.Context, wg *sync.WaitGroup, logger *logrus.Entry, port int) error {

	log = logger.WithField("module", "web")

	initTemplates()

	log.Debug("Starting Webserver...")

	http.HandleFunc("/perf/", perfHome)
	http.HandleFunc("/perf/feed", perfLog)
	http.HandleFunc("/favicon.ico", notFoundHandler)
	http.HandleFunc("/", home)

	server = &http.Server{
		Addr: ":" + strconv.Itoa(port),
	}

	go func() {
		wg.Add(1)
		defer wg.Done()
		log.Debug("Started Webserver.")
		err := server.ListenAndServe()
		if err != nil && err.Error() != "http: Server closed" {
			log.Errorf("Unable to start the Webserver: %v", err.Error())
		}
		log.Debug("Stopped Webserver.")
	}()

	go func() {
		select {
		case <-ctx.Done():
			log.Debug("Stopping Webserver...")
			webCtx, webCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer webCancel()
			err := server.Shutdown(webCtx)
			if err != nil {
				log.Errorf("Error shutting down Webserver: %v", err.Error())
			}
		}
	}()

	return nil
}

func initTemplates() {
	if templates == nil {
		templates = make(map[string]*template.Template)
	}

	templatesDir := path.Join("templates", "*.tmpl")

	templatePaths, err := filepath.Glob(templatesDir)
	if err != nil {
		log.Fatal("Error initializing HTML Templates", err)
	}

	log.Debugf("Loading %d templates from %v", len(templatePaths), templatesDir)

	for _, filePath := range templatePaths {
		name := strings.TrimSuffix(path.Base(filePath), ".tmpl")
		templates[name] = template.Must(template.ParseFiles(filePath))
	}
}

func home(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, r, "home", template.HTML("<a href=\"/perf/http:%2F%2Fwww.pgatour.com%2Ftest.json\">Perf TOUR</a>"))
}

func perfHome(w http.ResponseWriter, r *http.Request) {

	names, err := db.GetPerformanceBucketNames()
	if err != nil {
		errorHandler(w, r, fmt.Sprintf("Error getting bucket names. %v", err.Error()))
		return
	}

	args := make([]interface{}, len(names), len(names))
	for i, v := range names {
		values := url.Values{}
		values.Set("feed", v)
		values.Set("date", "today")

		args[i] = struct {
			Name string
			URL  template.URL
		}{
			v,
			template.URL("feed?" + values.Encode()),
		}
	}

	renderTemplate(w, r, "perfHome", args)
}

func perfLog(w http.ResponseWriter, r *http.Request) {

	requestValues := r.URL.Query()
	dateArg := requestValues.Get("date")
	url := requestValues.Get("feed")

	var date time.Time
	if strings.EqualFold(strings.TrimSpace(dateArg), "today") {
		date = time.Now()
	} else {
		var err error
		date, err = time.Parse("2006-01-02", dateArg)
		if err != nil {
			badRequestHandler(w, r)
			return
		}
	}

	perfRecs, err := db.GetPerformanceRecordsForDate(url, date)
	if err != nil {
		errorHandler(w, r, err.Error())
		return
	}

	if perfRecs == nil {
		notFoundHandler(w, r)
		return
	}

	year, month, day := date.Date()
	d := time.Date(year, month, day, 0, 0, 0, 0, date.Location())
	tom := d.Add(24 * time.Hour)

	templateData := make(map[string]interface{})
	templateData["FeedURL"] = url
	templateData["Date"] = date.Format("Mon Jan _2 2006")
	templateData["graphData"] = template.JS(buildGraphMapString(perfRecs))
	templateData["StartDate"] = template.JS(fmt.Sprintf("new Date(%d, %d, %d, 0, 0)", d.Year(), d.Month()-1, d.Day()))
	templateData["EndDate"] = template.JS(fmt.Sprintf("new Date(%d, %d, %d, 0, 0)", tom.Year(), tom.Month()-1, tom.Day()))

	renderTemplate(w, r, "perflog", templateData)
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
}

func badRequestHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusBadRequest)
}

func errorHandler(w http.ResponseWriter, r *http.Request, errorDesc string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "Server Error: %v", errorDesc)
}

func renderTemplate(w http.ResponseWriter, r *http.Request, name string, data interface{}) {

	tmpl, ok := templates[name]
	if !ok {
		errorHandler(w, r, fmt.Sprintf("No template found for name: %s", name))
	}

	log.Debugf("Rendering template name %s", tmpl.Name())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := tmpl.ExecuteTemplate(w, name+".tmpl", data)
	if err != nil {
		errorHandler(w, r, fmt.Sprintf("Unable to Excecute Template %v. Error: %v", name, err))
	}
}

func buildGraphMapString(perfRecs []db.PerformanceEntryResult) (result string) {

	delim := ""
	for i, v := range perfRecs {
		if i > 0 {
			delim = ", "
		}
		result += fmt.Sprintf("%v[new Date(%d000), %d, %d]", delim, v.CheckTime.Unix(), v.Duration, v.Size)
	}
	return
}
