package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
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
	r.Handle("/js/{rest}", http.StripPrefix("/js/", http.FileServer(http.Dir("web/js"))))
	r.Handle("/css/{rest}", http.StripPrefix("/css/", http.FileServer(http.Dir("web/css"))))
	r.Handle("/fonts/{rest}", http.StripPrefix("/fonts/", http.FileServer(http.Dir("web/fonts"))))
	r.HandleFunc("/app/{app}/", appHome)
	r.HandleFunc("/app/{app}/{endpoint}/", endpointHome)
	r.HandleFunc("/app/{app}/{endpoint}/result", endpointResult)
	r.HandleFunc("/app/{app}/{endpoint}/results", endpointResults)
	r.HandleFunc("/app/{app}/{endpoint}/resultsdiff", endpointResultsDiff)
	r.HandleFunc("/app/{app}/{endpoint}/resultsinvalid", endpointResultsInvalid)
	r.HandleFunc("/app/{app}/{endpoint}/performance", endpointPerformance)
	r.HandleFunc("/app/{app}/{endpoint}/replay", endpointReplay)
	r.HandleFunc("/app/{app}/{endpoint}/diff", endpointDiff)

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
	if templates != nil && !configuration.WebDevMode {
		return
	}

	if templates == nil {
		templates = make(map[string]*template.Template)
	}

	templatesDir := path.Join("templates", "*.tmpl")

	templatePaths, err := filepath.Glob(templatesDir)
	if err != nil {
		webLog.Fatal("Error initializing HTML Templates", err)
	}
	templateHelperPath := path.Join("templates", "helper.tmpl")

	funcMap := template.FuncMap{
		"FormatDuration": formatDuration,
		"Comma":          humanize.Comma,
		"Bytes":          func(b int64) string { return humanize.Bytes(uint64(b)) },
	}
	_ = uint64(34)
	webLog.Debugf("Loading %d templates from %v", len(templatePaths), templatesDir)

	for _, filePath := range templatePaths {
		name := strings.TrimSuffix(path.Base(filePath), ".tmpl")
		t := template.New(name).Funcs(funcMap)
		templates[name] = template.Must(t.ParseFiles(filePath, templateHelperPath))
	}
}

func formatDuration(d time.Duration) int64 {
	return int64(d / time.Millisecond)
}

func home(w http.ResponseWriter, r *http.Request) {
	initTemplates()

	templateData := make(map[string]interface{})
	templateData["Applications"] = applications

	renderTemplate(w, r, "home", templateData)
}

func appHome(w http.ResponseWriter, r *http.Request) {
	initTemplates()

	vars := mux.Vars(r)

	app := configuration.getApplication(vars["app"])
	if app == nil {
		notFoundHandler(w, r)
		return
	}

	templateData := make(map[string]interface{})
	templateData["Applications"] = applications
	templateData["Application"] = app

	renderTemplate(w, r, "appHome", templateData)
}

func endpointHome(w http.ResponseWriter, r *http.Request) {
	initTemplates()

	found, app, endpoint := getAppEndpoint(w, r)
	if !found {
		notFoundHandler(w, r)
		return
	}

	templateData := make(map[string]interface{})
	urls := []string{}
	recentResults := make(map[string][]EndpointResult)
	templateData["Applications"] = applications
	templateData["Application"] = app
	templateData["Endpoint"] = endpoint

	app.rwMu.RLock()
	if endpoint.Dynamic {
		for _, url := range endpoint.CurrentURLs {
			urls = append(urls, url)
			res, _ := GetLastNEndpointResult(app.Key, endpoint.Key, url, 10)
			if res != nil {
				recentResults[url] = res
			}
		}
	} else {
		urls = append(urls, endpoint.URL)
		recentResults[endpoint.URL], _ = GetLastNEndpointResult(app.Key, endpoint.Key, endpoint.URL, 10)
	}
	app.rwMu.RUnlock()

	templateData["URLS"] = urls
	templateData["Results"] = recentResults

	renderTemplate(w, r, "endpointHome", templateData)
}

func endpointResult(w http.ResponseWriter, r *http.Request) {
	initTemplates()
	found, app, endpoint := getAppEndpoint(w, r)
	if !found {
		notFoundHandler(w, r)
		return
	}

	date, ok := getDate(r, time.RFC3339)
	if !ok {
		notFoundHandler(w, r)
		return
	}

	app.rwMu.RLock()
	url := getURL(endpoint, r)

	epr, err := GetEndpointResult(app.Key, endpoint.Key, url, date)
	if err != nil {
		errorHandler(w, r, err.Error())
		return
	} else if epr == nil {
		notFoundHandler(w, r)
		return
	}
	app.rwMu.RUnlock()

	templateData := make(map[string]interface{})
	templateData["Applications"] = applications
	templateData["Application"] = app
	templateData["Endpoint"] = endpoint
	templateData["FeedURL"] = url
	templateData["Result"] = epr

	renderTemplate(w, r, "endpointResult", templateData)
}

func endpointResults(w http.ResponseWriter, r *http.Request) {
	initTemplates()
	found, app, endpoint := getAppEndpoint(w, r)
	if !found {
		notFoundHandler(w, r)
		return
	}

	date, ok := getDate(r, "2006-01-02")
	if !ok {
		badRequestHandler(w, r)
		return
	}

	app.rwMu.RLock()
	url := getURL(endpoint, r)

	results, _ := GetEndpointResultsForDate(app.Key, endpoint.Key, url, date)
	app.rwMu.RUnlock()

	templateData := make(map[string]interface{})
	templateData["Applications"] = applications
	templateData["Application"] = app
	templateData["Endpoint"] = endpoint
	templateData["Results"] = results
	templateData["Date"] = date.Format("Mon Jan _2 2006")
	templateData["NextDate"] = date.Add(24 * time.Hour)
	templateData["PrevDate"] = date.Add(-24 * time.Hour)
	templateData["FeedURL"] = url

	renderTemplate(w, r, "endpointResults", templateData)
}

func endpointResultsDiff(w http.ResponseWriter, r *http.Request) {
	initTemplates()
	found, app, endpoint := getAppEndpoint(w, r)
	if !found {
		notFoundHandler(w, r)
		return
	}

	app.rwMu.RLock()
	url := getURL(endpoint, r)

	results, _ := GetLastNDiffEndpointResult(app.Key, endpoint.Key, url, 100)
	app.rwMu.RUnlock()

	templateData := make(map[string]interface{})
	templateData["Applications"] = applications
	templateData["Application"] = app
	templateData["Endpoint"] = endpoint
	templateData["Results"] = results
	templateData["FeedURL"] = url
	templateData["FilterName"] = "Diffs"

	renderTemplate(w, r, "endpointResultsAlt", templateData)
}

func endpointResultsInvalid(w http.ResponseWriter, r *http.Request) {
	initTemplates()
	found, app, endpoint := getAppEndpoint(w, r)
	if !found {
		notFoundHandler(w, r)
		return
	}

	app.rwMu.RLock()
	url := getURL(endpoint, r)

	results, _ := GetLastNInvalidEndpointResult(app.Key, endpoint.Key, url, 100)
	app.rwMu.RUnlock()

	templateData := make(map[string]interface{})
	templateData["Applications"] = applications
	templateData["Application"] = app
	templateData["Endpoint"] = endpoint
	templateData["Results"] = results
	templateData["FeedURL"] = url
	templateData["FilterName"] = "Invalid Results"

	renderTemplate(w, r, "endpointResultsAlt", templateData)
}

func endpointPerformance(w http.ResponseWriter, r *http.Request) {
	initTemplates()
	found, app, endpoint := getAppEndpoint(w, r)
	if !found {
		notFoundHandler(w, r)
		return
	}

	date, ok := getDate(r, "2006-01-02")
	if !ok {
		badRequestHandler(w, r)
		return
	}

	app.rwMu.RLock()
	url := getURL(endpoint, r)

	perfRecs, err := GetPerformanceRecordsForDate(app.Key, endpoint.Key, url, date)
	app.rwMu.RUnlock()

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
	templateData["Applications"] = applications
	templateData["Application"] = app
	templateData["Endpoint"] = endpoint
	templateData["FeedURL"] = url
	templateData["Date"] = date.Format("Mon Jan _2 2006")
	templateData["graphData"] = template.JS(buildGraphMapString(perfRecs))
	templateData["StartDate"] = template.JS(fmt.Sprintf("new Date(%d, %d, %d, 0, 0)", d.Year(), d.Month()-1, d.Day()))
	templateData["EndDate"] = template.JS(fmt.Sprintf("new Date(%d, %d, %d, 0, 0)", tom.Year(), tom.Month()-1, tom.Day()))
	templateData["NextDate"] = date.Add(24 * time.Hour)
	templateData["PrevDate"] = date.Add(-24 * time.Hour)

	renderTemplate(w, r, "endpointPerformance", templateData)
}

func endpointReplay(w http.ResponseWriter, r *http.Request) {
	found, app, endpoint := getAppEndpoint(w, r)
	if !found {
		notFoundHandler(w, r)
		return
	}

	date, ok := getDate(r, time.RFC3339)
	if !ok {
		badRequestHandler(w, r)
		return
	}

	app.rwMu.RLock()
	url := getURL(endpoint, r)

	epr, err := GetEndpointResult(app.Key, endpoint.Key, url, date)
	app.rwMu.RUnlock()

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

func endpointDiff(w http.ResponseWriter, r *http.Request) {
	initTemplates()
	found, app, endpoint := getAppEndpoint(w, r)
	if !found {
		notFoundHandler(w, r)
		return
	}

	date, ok := getDate(r, time.RFC3339)
	if !ok {
		badRequestHandler(w, r)
		return
	}

	app.rwMu.RLock()
	url := getURL(endpoint, r)

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

	oldEpr, err := GetEndpointResultPrev(app.Key, endpoint.Key, url, date)
	app.rwMu.RUnlock()
	if err != nil {
		errorHandler(w, r, err.Error())
		return
	}
	if epr == nil {
		log.Errorf("No result Found:")
		notFoundHandler(w, r)
		return
	}

	var oldPretty bytes.Buffer
	var newPretty bytes.Buffer
	if oldEpr != nil {
		json.Indent(&oldPretty, oldEpr.Body, "", "  ")
	}
	json.Indent(&newPretty, epr.Body, "", "  ")

	templateData := make(map[string]interface{})
	templateData["Applications"] = applications
	templateData["Application"] = app
	templateData["Endpoint"] = endpoint
	templateData["OldBody"] = string(oldPretty.Bytes())
	templateData["NewBody"] = string(newPretty.Bytes())

	renderTemplate(w, r, "endpointDiff", templateData)
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

func renderTemplate(w http.ResponseWriter, r *http.Request, name string, data map[string]interface{}) {

	applicationsRWMu.RLock()
	defer applicationsRWMu.RUnlock()
	for _, app := range applications {
		app.rwMu.RLock()
		defer app.rwMu.RUnlock()
	}

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
	url := r.URL.Query().Get("feed")
	if url == "" && !e.Dynamic {
		url = e.URL
	}
	return url
}

func getDate(r *http.Request, format string) (time.Time, bool) {
	return getDateFromField(r, format, "date")
}

func getDateFromField(r *http.Request, format string, dateField string) (time.Time, bool) {
	dateArg := r.URL.Query().Get(dateField)
	var date time.Time
	if strings.EqualFold(strings.TrimSpace(dateArg), "today") {
		return time.Now(), true
	}

	var err error
	date, err = time.Parse(format, dateArg)
	if err != nil {
		log.Errorf("Error parsing date: %v", err.Error())
		return time.Now(), false
	}
	return date, true
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
