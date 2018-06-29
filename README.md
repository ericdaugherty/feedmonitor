# FeedMonitor
FeedMonitor is a tool to track the status and changes in HTTP(S) feeds (JSON or other).

This is primarily useful when you want to monitor that status and values of APIs provided via HTTP(S). Currently FeedMonitor supports feed data in JSON.

To use, compile the application using Go and edit the feedmon.yaml file.

FeedMonitor uses a local database file (feedmon.db) and a set of local git repositories to store all persistent data. The database filename is not configurable, but the directories to store the get repos is defined in feedmon.yaml.

The configuration files are stored in a config directory, (cfg by default) which can be defined in feedmon.yaml.

A sample config file is provied in the cfg directory and is documented. You can define as many config files as you wish, and each file will represent a seperate set of URLs to monitor.

The three main compnoents are:

## Feeds

Feed definitions can be relatively complex. They support:
- Different METHOD types
- Request Bodies
- HTTP Headers
- Ability to ignore redirects
- Check Interval (in minutes)

Feeds can also use date from other feeds.  For example, if one feed returns a JSON list of IDs, you can define a second feed to check a unique URL for each of the provided IDs. An example may be a feed like this:

```
- key: mainfeed
   name: MainPlayerList
   url: http://www.example.com/data/players.json
   checkinterval: 3
```

Assume that the mainfeed feed returns a JSON array of tournaments with an id value. You could define the second feed as follows:
```
 - key: secondaryfeed
   name: Secondary Feed
   url: "{{range .mainfeed.data.tournaments}}http://www.example.com/data/{{.id}}/secondary.json|||{{end}}"
   dynamic: yes
   checkinterval: 3
```

Note that the dynamic field is set to true. This indicates that the URL attirbute should be evaluated and may produce more than one URL to check. In this case, it would iterate over the JSON returned by the 'mainfeed' feed, and naviaget into the data object for a tournements array, where each child has an id attribute. That ID would then be used in the URL for the 'secondaryfeed' feed to check.

Each feed allows you to define specific validators and notifiers, and they also inherit the 'default' validators and notifiers.

## Validators

Validators can be run on each feed. There are several useful validators provided by default:

- Size: validates the size (min or max) of the body of the result.
- Status: Define one or more expected valid status results.
- JSON: Validates that well-formed json is returned
- JSONData: Allows detailed validation of specific data fields within a json response, including navigating and iterating arrays. The sample config file provides a good intro to the options availabile.

## Notifiers

StdErr HipChat, and MS Teams notifiers are provided by default. The interace is simple and additional notifiers can be added easily.

## Web Interface

All information can be queried using the web interface.

You can access the web interface by default at: http://localhost:8080 by default
