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
	ConfigPath     string `short:"c" long:"config" description:"The path to the configuration file." default:"feedmon.yaml"`
	LogLevel       string `short:"l" long:"log-level" description:"The minimum log level to output." choice:"debug" choice:"info" choice:"warn" choice:"error"`
	WebDevelopment bool   `short:"w" long:"webdev" description:"Enable Development Mode for the web tier. Templates will be reloaded on each request."`
}

var options Options
var configuration = &Configuration{}
var applications []*Application
var applicationsRWMu = &sync.RWMutex{}
var mainWg = &sync.WaitGroup{}
var log *logrus.Entry

var parser = flags.NewParser(&options, flags.Default)

func main() {

	// Parse command line flags.
	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			fmt.Println("Error Parsing Flags", err)
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

	StartWatchingConfigDirectory()

	applicationsRWMu.RLock()
	for _, a := range applications {
		a.startFeedMonitor(mainWg)
	}
	applicationsRWMu.RUnlock()

	select {
	case <-c:
		log.Info("System Interrupt Received. Shutting Down.")
		applicationsRWMu.RLock()
		for _, app := range applications {
			app.stopFeedMonitor()
		}
		applicationsRWMu.RUnlock()
		cancel()
	case <-ctx.Done():
		fmt.Println("Done")
	}

	if waitTimeout(mainWg, 10*time.Second) {
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
		logfile, err := os.OpenFile(configuration.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Printf("Error initialializing log file: %v - %v\r\n", configuration.LogFile, err)
			os.Exit(1)
		}
		logrus.SetOutput(logfile)
	}

	return logrus.WithFields(logrus.Fields{"module": "core"})
}
