package main

import (
	"os"
	"strings"

	fsnotify "gopkg.in/fsnotify.v1"
)

// StartWatchingConfigDirectory starts a goroutine that monitors the application config directory for changes.
func StartWatchingConfigDirectory() {
	log.Debugf("Starting Config Directory FileWatcher")
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Errorf("Failed to start Config Directory Watcher. %v", err)
		return
	}

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				switch event.Op {
				case fsnotify.Chmod:
					reloadConfigFile(event)
				case fsnotify.Create:
					loadNewConfigFile(event)
				case fsnotify.Remove:
					removeConfigFile(event)
				}
			case err := <-watcher.Errors:
				log.Errorf("FileWatcher Error. %v", err.Error())
			}
		}
	}()

	watcher.Add(configuration.AppConfigDir)
}

func reloadConfigFile(event fsnotify.Event) {
	applicationsRWMu.Lock()
	for i, app := range applications {
		if app.FileName == event.Name {

			stat, err := os.Stat(app.FileName)
			if err != err {
				log.Errorf("Error accessing file information for file %v. %v", app.FileName, err)
				applicationsRWMu.Unlock()
				return
			}

			if stat.ModTime().Equal(app.LastModified) {
				log.Info("Ignoring Chmod notification. Last load time and last modified times are equal. %v", app.LastModified)
				applicationsRWMu.Unlock()
				return
			}

			updatedApp := configuration.initializeApplication(app.FileName)
			app.stopFeedMonitor()
			applications[i] = updatedApp
			updatedApp.startFeedMonitor(mainWg)
			applicationsRWMu.Unlock()
			return
		}
	}
	// App was modified but we have not loaded it, so treat it as a new file.
	applicationsRWMu.Unlock()
	loadNewConfigFile(event)
}

func loadNewConfigFile(event fsnotify.Event) {
	if !strings.HasSuffix(event.Name, ".") && strings.HasSuffix(event.Name, ".yaml") {
		log.Debugf("New Config File %v found. Attempting to load.", event.Name)
		newApp := configuration.initializeApplication(event.Name)
		if newApp == nil {
			log.Errorf("Unable to load configuration from new file %v. See previous error.", event.Name)
		} else {
			existingApp := configuration.getApplication(newApp.Key)
			if existingApp != nil {
				log.Errorf("Attempted to load new configuration file, but the application key %v is already in use. Ignoring file %v.", newApp.Key, event.Name)
				return
			}
			applicationsRWMu.Lock()
			applications = append(applications, newApp)
			newApp.startFeedMonitor(mainWg)
			applicationsRWMu.Unlock()
		}
	}
}

func removeConfigFile(event fsnotify.Event) {
	applicationsRWMu.Lock()
	defer applicationsRWMu.Unlock()
	for i, app := range applications {
		if app.FileName == event.Name {
			app.stopFeedMonitor()
			applications = append(applications[:i], applications[i+1:]...)
			return
		}
	}
}
