// A port of the ruby 'shotgun' library
package main

import (
	"flag"
	"fmt"
	"github.com/DanielHeath/shotgun-go/webprocess"
	"io/ioutil"
	"launchpad.net/goyaml"
	logger "log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
)

var log *logger.Logger

var port int
var rawurl, checkCmd, buildCmd, runCmd, configFile string
var proxyUrl *url.URL

var wp *webprocess.WebProcess

type YmlConfig struct {
	Env      []map[string]string
	Port     int
	Url      string
	CheckCmd string
	BuildCmd string
	RunCmd   string
}

func init() {
	log = logger.New(os.Stdout, "Shotgun: ", 0)

	flag.IntVar(&port, "p", 8009, "Shorthand for --port")
	flag.IntVar(&port, "port", 8009, "The port for shotgun to listen on")
	flag.StringVar(&rawurl, "u", "", "Shorthand for --url")
	flag.StringVar(&rawurl, "url", "", "The url to proxy for")
	flag.StringVar(&checkCmd, "checkCmd", "", "Command to check if build is required. Command should exit 1 for a rebuild, 0 for no rebuild. (required)")
	flag.StringVar(&buildCmd, "buildCmd", "", "Command to build the executable (required)")
	flag.StringVar(&runCmd, "runCmd", "", "Command to run the executable (required)")
	flag.StringVar(&configFile, "config", "", "Config file")
	flag.Parse()

	// try loading config file if no parameters have been given
	if rawurl == "" && checkCmd == "" && buildCmd == "" && runCmd == "" && configFile == "" {
		configFile = ".shotgun-go"
	}

	if configFile != "" {
		configBytes, err := ioutil.ReadFile(configFile)
		if err == nil {
			ymlconfig := YmlConfig{}
			err = goyaml.Unmarshal(configBytes, &ymlconfig)
			if err != nil {
				panic(err)
			}
			log.Println("Read config " + configFile)

			rawurl = ymlconfig.Url
			port = ymlconfig.Port
			checkCmd = ymlconfig.CheckCmd
			buildCmd = ymlconfig.BuildCmd
			runCmd = ymlconfig.RunCmd

			for _, envmap := range ymlconfig.Env {
				for key, value := range envmap {
					log.Println("Enironment set " + key + ": " + value)
					os.Setenv(key, value)
				}
			}
		}
	}

	var err error
	proxyUrl, err = url.Parse(rawurl)

	if rawurl == "" || checkCmd == "" || buildCmd == "" || runCmd == "" || err != nil || len(flag.Args()) != 0 {
		fmt.Fprintf(os.Stderr, "Shotgun is a reverse proxy for hot-reloading code.\n")
		fmt.Fprintf(os.Stderr, `Usage: %s [options] -checkCmd="" -buildCmd="" -runCmd=""`+"\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Example Usage:
shotgun -u http://localhost:8008 -p 8010 -checkCmd='exit `+"`find -name *.go -newer ./bin/myapp | wc -l`"+`' -buildCmd="go build -o ./bin/myapp myapp" -runCmd="./bin/myapp"

`)

		if rawurl != "" && err != nil {
			fmt.Fprintf(os.Stderr, "URL must be a valid url: %s", err)
		}
		os.Exit(-1)
	}
}

func signalHandler(c chan os.Signal) {
	// Block until a signal is received.
	<-c
	wp.Stop()
	os.Exit(0)
}

func main() {
	log.Printf("Starting reverse proxy on http://localhost:%d for %s\n", port, proxyUrl.String())
	log.Println("Check command: " + checkCmd)
	log.Println("Build command: " + buildCmd)
	log.Println("Run command: " + runCmd)

	wp = webprocess.NewWebProcess(
		checkCmd,
		buildCmd,
		runCmd,
		proxyUrl,
		log,
	)

	// Listen for os signals
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	go signalHandler(c)

	http.HandleFunc("/", wp.ServeHTTP)
	panic(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}
