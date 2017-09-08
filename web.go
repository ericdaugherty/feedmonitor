package main

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
	r.HandleFunc("/app/{app}/{endpoint}/performance", endpointPerformance)

	r.HandleFunc("/perf/", perfHome)
	r.HandleFunc("/perf/feed", perfLog)
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

	args := struct {
		Application *Application
		Endpoint    *Endpoint
	}{
		app,
		endpoint,
	}

	renderTemplate(w, r, "endpointHome", args)
}

func endpointPerformance(w http.ResponseWriter, r *http.Request) {
	found, app, endpoint := getAppEndpoint(w, r)
	if !found {
		notFoundHandler(w, r)
		return
	}

	requestValues := r.URL.Query()
	dateArg := requestValues.Get("date")
	var url string
	if endpoint.Dynamic {
		url = requestValues.Get("feed")
	} else {
		url = endpoint.URL
	}

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

	perfRecs, err := GetPerformanceRecordsForDate(url, date)
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

func perfHome(w http.ResponseWriter, r *http.Request) {

	names, err := GetPerformanceBucketNames()
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

	perfRecs, err := GetPerformanceRecordsForDate(url, date)
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
