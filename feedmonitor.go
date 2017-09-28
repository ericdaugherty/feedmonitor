package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/sirupsen/logrus"
)

// Options defines the valid command line parameters/flags.
type Options struct {
	ConfigPath string `short:"c" long:"config" description:"The path to the configuration file." default:"feedmon.yaml"`
	LogLevel   string `short:"l" long:"log-level" description:"The minimum log level to output." choice:"debug" choice:"info" choice:"warn" choice:"error"`
}

var options Options
var configuration = &Configuration{}
var applications []*Application
var log *logrus.Entry
var shutdown = false

var parser = flags.NewParser(&options, flags.Default)

func main() {

	// Parse command line flags.
	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			os.Exit(1)
		}
	}

	configuration = loadConfigFile()
	configuration.initialize()

	log = initializeLogger()

	log.Info("FeedMonitor Starting...")

	configuration.initializeApplications()

	err := StartDatabase("feedmon.db", log)
	if err != nil {
		log.Fatalf("Unable to launch FeedMonitor.  Error initializing the database. %v", err.Error())
	}

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	helperContext, helperCancel := context.WithCancel(context.Background())

	var mainWg sync.WaitGroup
	var helperWg sync.WaitGroup

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	defer func() {
		signal.Stop(c)
		cancel()
	}()

	StartWebserver(ctx, &helperWg, log, configuration.WebPort)

	NotificationChannel = StartNotificationHandler(helperContext, &helperWg)

	ResultLogChannel = StartResultWriter(helperContext, &helperWg)

	if len(applications) == 0 {
		log.Fatalf("No applications found. Exiting.")
	}

	for _, a := range applications {
		go startFeedMonitor(ctx, &mainWg, a)
	}

	select {
	case <-c:
		log.Info("System Interrupt Received. Shutting Down.")
		shutdown = true
		cancel()
	case <-ctx.Done():
		fmt.Println("Done")
	}

	if waitTimeout(&mainWg, 10*time.Second) {
		log.Info("App Threads Shutdown after TIMEOUT.")
	} else {
		log.Info("App Threads Shutdown Cleanly.")
	}
	helperCancel()

	if waitTimeout(&helperWg, 10*time.Second) {
		log.Info("Helper Threads Shutdown after TIMEOUT.")
	} else {
		log.Info("Helper Threads Shutdown Cleanly.")
	}
}

func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return false // completed normally
	case <-time.After(timeout):
		return true // timed out
	}
}

func initializeLogger() *logrus.Entry {
	var logLevel logrus.Level
	switch strings.ToLower(configuration.LogLevel) {
	case "debug":
		logLevel = logrus.DebugLevel
	case "info":
		logLevel = logrus.InfoLevel
	case "warn":
		logLevel = logrus.WarnLevel
	case "error":
		logLevel = logrus.ErrorLevel
	default:
		logLevel = logrus.DebugLevel
	}
	logrus.SetLevel(logLevel)

	if len(configuration.LogFile) > 0 && !strings.EqualFold("console", configuration.LogFile) {
		logfile, err := os.OpenFile(configuration.LogFile, os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			fmt.Printf("Error initialializing log file: %v - %v\r\n", configuration.LogFile, err)
			os.Exit(1)
		}
		logrus.SetOutput(logfile)
	}

	return logrus.WithFields(logrus.Fields{"module": "core"})
}

func startFeedMonitor(ctx context.Context, wg *sync.WaitGroup, app *Application) {

	log := log.WithFields(logrus.Fields{
		"module": "feedmonitor",
		"app":    app.Key,
	})

	ticker := time.NewTicker(1 * time.Second)

	go func() {
		log.Debug("Started Feed Checker.")

		wg.Add(1)
		defer wg.Done()

		var data = make(map[string]interface{})
		for {
			select {
			case <-ticker.C:
				for _, e := range app.Endpoints {
					if shutdown {
						log.Debug("Shutting down Feed Checker.")
						return
					}
					if e.shouldCheckNow() {
						e.scheduleNextCheck()
						if e.Dynamic {
							urls, err := e.parseURLs(data)
							if err != nil {
								log.Errorf("Error parsing URL: %v Error: %v", e.URL, err.Error())
							}
							for _, url := range urls {
								fetchEndpoint(app, e, url)
							}
						} else {
							data[e.Key], _ = fetchEndpoint(app, e, e.URL)
						}
					}
				}

			case <-ctx.Done():
				log.Debug("Shutting down Feed Checker.")
				return
			}
		}
	}()
}
