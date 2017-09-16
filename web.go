package main

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

var webLog *logrus.Entry
var server *http.Server

var tmpl *template.Template
var templates map[string]*template.Template

// StartWebserver initializes the webserver and starts the listener.
func StartWebserver(ctx context.Context, wg *sync.WaitGroup, logger *logrus.Entry, port int) error {

	webLog = logger.WithField("module", "web")

	initTemplates()

	webLog.Debug("Starting Webserver...")

	r := mux.NewRouter()

	r.HandleFunc("/", home)
	r.HandleFunc("/app/{app}/", appHome)
	r.HandleFunc("/app/{app}/{endpoint}/", endpointHome)
	r.HandleFunc("/app/{app}/{endpoint}/results", endpointResults)
	r.HandleFunc("/app/{app}/{endpoint}/performance", endpointPerformance)
	r.HandleFunc("/app/{app}/{endpoint}/replay", replayEndpointResult)

	r.HandleFunc("/favicon.ico", notFoundHandler)
	http.Handle("/", r)

	server = &http.Server{
		Addr: ":" + strconv.Itoa(port),
	}

	go func() {
		wg.Add(1)
		defer wg.Done()
		webLog.Debug("Started Webserver.")
		err := server.ListenAndServe()
		if err != nil && err.Error() != "http: Server closed" {
			webLog.Errorf("Unable to start the Webserver: %v", err.Error())
		}
		webLog.Debug("Stopped Webserver.")
	}()

	go func() {
		select {
		case <-ctx.Done():
			webLog.Debug("Stopping Webserver...")
			webCtx, webCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer webCancel()
			err := server.Shutdown(webCtx)
			if err != nil {
				webLog.Errorf("Error shutting down Webserver: %v", err.Error())
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
		webLog.Fatal("Error initializing HTML Templates", err)
	}

	webLog.Debugf("Loading %d templates from %v", len(templatePaths), templatesDir)

	for _, filePath := range templatePaths {
		name := strings.TrimSuffix(path.Base(filePath), ".tmpl")
		templates[name] = template.Must(template.ParseFiles(filePath))
	}
}

func home(w http.ResponseWriter, r *http.Request) {

	args := struct {
		Applications []*Application
	}{
		configuration.Applications,
	}

	renderTemplate(w, r, "home", args)
}

func appHome(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	app := configuration.getApplication(vars["app"])
	if app == nil {
		notFoundHandler(w, r)
		return
	}

	renderTemplate(w, r, "appHome", app)
}

func endpointHome(w http.ResponseWriter, r *http.Request) {
	found, app, endpoint := getAppEndpoint(w, r)
	if !found {
		notFoundHandler(w, r)
		return
	}

	recentResults, _ := GetLastNEndpointResult(app.Key, endpoint.Key, endpoint.URL, 10)

	args := struct {
		Application   *Application
		Endpoint      *Endpoint
		RecentResults []EndpointResult
	}{
		app,
		endpoint,
		recentResults,
	}

	renderTemplate(w, r, "endpointHome", args)
}

func endpointResults(w http.ResponseWriter, r *http.Request) {
	found, app, endpoint := getAppEndpoint(w, r)
	if !found {
		notFoundHandler(w, r)
		return
	}

	url := getURL(endpoint, r)
	dv, date := getDate(r, "2006-01-02")
	if !dv {
		badRequestHandler(w, r)
		return
	}

	results, _ := GetEndpointResultsForDate(app.Key, endpoint.Key, url, date)

	args := struct {
		Application *Application
		Endpoint    *Endpoint
		Results     []EndpointResult
		Date        time.Time
		NextDate    time.Time
		PrevDate    time.Time
		Feed        string
	}{
		app,
		endpoint,
		results,
		date,
		date.Add(24 * time.Hour),
		date.Add(-24 * time.Hour),
		url,
	}

	renderTemplate(w, r, "endpointResults", args)
}

func endpointPerformance(w http.ResponseWriter, r *http.Request) {
	found, app, endpoint := getAppEndpoint(w, r)
	if !found {
		notFoundHandler(w, r)
		return
	}

	url := getURL(endpoint, r)
	dv, date := getDate(r, "2006-01-02")
	if !dv {
		badRequestHandler(w, r)
		return
	}

	perfRecs, err := GetPerformanceRecordsForDate(app.Key, endpoint.Key, url, date)
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
	templateData["Application"] = app
	templateData["Endpoint"] = endpoint
	templateData["FeedURL"] = url
	templateData["Date"] = date.Format("Mon Jan _2 2006")
	templateData["graphData"] = template.JS(buildGraphMapString(perfRecs))
	templateData["StartDate"] = template.JS(fmt.Sprintf("new Date(%d, %d, %d, 0, 0)", d.Year(), d.Month()-1, d.Day()))
	templateData["EndDate"] = template.JS(fmt.Sprintf("new Date(%d, %d, %d, 0, 0)", tom.Year(), tom.Month()-1, tom.Day()))

	renderTemplate(w, r, "endpointPerformance", templateData)
}

func replayEndpointResult(w http.ResponseWriter, r *http.Request) {
	found, app, endpoint := getAppEndpoint(w, r)
	if !found {
		notFoundHandler(w, r)
		return
	}

	url := getURL(endpoint, r)
	dv, date := getDate(r, time.RFC3339)
	if !dv {
		badRequestHandler(w, r)
		return
	}

	epr, err := GetEndpointResult(app.Key, endpoint.Key, url, date)
	if err != nil {
		errorHandler(w, r, err.Error())
		return
	}
	if epr == nil {
		log.Errorf("No result Found:")
		notFoundHandler(w, r)
		return
	}

	renderEndpointReplay(w, r, epr)
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	webLog.Debugf("Rendering 404 for URL %s", r.RequestURI)
	w.WriteHeader(http.StatusNotFound)
}

func badRequestHandler(w http.ResponseWriter, r *http.Request) {
	webLog.Debugf("Rendering 400 for URL %s", r.RequestURI)
	w.WriteHeader(http.StatusBadRequest)
}

func errorHandler(w http.ResponseWriter, r *http.Request, errorDesc string) {
	webLog.Debugf("Rendering 500 for URL %s", r.RequestURI)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "Server Error: %v", errorDesc)
}

func renderTemplate(w http.ResponseWriter, r *http.Request, name string, data interface{}) {

	tmpl, ok := templates[name]
	if !ok {
		errorHandler(w, r, fmt.Sprintf("No template found for name: %s", name))
	}

	webLog.Debugf("Rendering template name %s", tmpl.Name())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := tmpl.ExecuteTemplate(w, name+".tmpl", data)
	if err != nil {
		errorHandler(w, r, fmt.Sprintf("Unable to Excecute Template %v. Error: %v", name, err))
	}
}

func renderEndpointReplay(w http.ResponseWriter, r *http.Request, epr *EndpointResult) {

	for k, v := range epr.Headers {
		for _, kv := range v {
			w.Header().Add(k, kv)
		}
	}
	w.Write(epr.Body)
}

func getAppEndpoint(w http.ResponseWriter, r *http.Request) (bool, *Application, *Endpoint) {
	vars := mux.Vars(r)

	app := configuration.getApplication(vars["app"])
	if app == nil {
		return false, nil, nil
	}

	endpoint := app.getEndpoint(vars["endpoint"])
	if endpoint == nil {
		return false, app, nil
	}

	return true, app, endpoint
}

func getURL(e *Endpoint, r *http.Request) string {
	if e.Dynamic {
		return r.URL.Query().Get("feed")
	}
	return e.URL
}

func getDate(r *http.Request, format string) (bool, time.Time) {
	dateArg := r.URL.Query().Get("date")
	var date time.Time
	if strings.EqualFold(strings.TrimSpace(dateArg), "today") {
		return true, time.Now()
	}

	var err error
	date, err = time.Parse(format, dateArg)
	if err != nil {
		log.Errorf("Error parsing date: %v", err.Error())
		return false, time.Now()
	}
	return true, date
}

func buildGraphMapString(perfRecs []PerformanceEntryResult) (result string) {

	delim := ""
	for i, v := range perfRecs {
		if i > 0 {
			delim = ", "
		}
		result += fmt.Sprintf("%v[new Date(%d000), %d, %d]", delim, v.CheckTime.Unix(), v.Duration, v.Size)
	}
	return
}
