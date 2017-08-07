package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/ericdaugherty/feedmonitor/db"
	"github.com/sirupsen/logrus"
)

// TODO: Change to html/template - but need to esacpe javascript code generation using text/template or other mechanism.

var log *logrus.Entry
var server *http.Server

var tmpl *template.Template

// StartWebserver initializes the webserver and starts the listener.
func StartWebserver(ctx context.Context, wg *sync.WaitGroup, logger *logrus.Entry, port int) error {

	log = logger.WithField("module", "web")

	tmpl = template.Must(template.ParseFiles("templates/perflog.html"))

	log.Debug("Starting Webserver...")

	http.HandleFunc("/perf/", perfHome)
	http.HandleFunc("/perf/url/", perfLog)
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

func home(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "<h1>Hello World</h1>")
	fmt.Fprint(w, "<br/>")
	fmt.Fprint(w, "<a href=\"/perf/http:%2F%2Fwww.pgatour.com%2Ftest.json\">Perf TOUR</a>")
}

func perfHome(w http.ResponseWriter, r *http.Request) {
	names, err := db.GetPerformanceBucketNames()
	if err != nil {
		fmt.Fprintf(w, "Error getting bucket names. %v", err.Error())
		return
	}

	fmt.Fprint(w, "<html><body>Performance Logs:<br/>")

	for _, v := range names {
		fmt.Fprintf(w, `<a href="url/%v">%v</a><br/>`, v, v)
	}

	fmt.Fprint(w, "</body></html>")
}

func perfLog(w http.ResponseWriter, r *http.Request) {

	url := strings.TrimPrefix(r.RequestURI, "/perf/url/")

	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = strings.Replace(url, ":/", "://", 1)
	}

	perfRecs, err := db.GetPerformanceRecords(url)
	if err != nil {
		errorHandler(w, r, err.Error())
		return
	}

	if perfRecs == nil || len(perfRecs) == 0 {
		notFoundHandler(w, r)
		return
	}

	// body := ""

	// body += fmt.Sprintf("Perf Log, URL: %v<br/>", url)
	// for _, v := range perfRecs {
	// 	body += fmt.Sprintf("Time: %v, Duration %d ms, Size: %d bytes.<br/>", v.CheckTime, v.Duration, v.Size)
	// }

	err = tmpl.Execute(w, perfRecs)
	if err != nil {
		log.Errorln("Error executing Template:", err)
	}
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
}

func errorHandler(w http.ResponseWriter, r *http.Request, errorDesc string) {
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "Server Error: %v", errorDesc)
}
