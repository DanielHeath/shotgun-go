// A port of the ruby 'shotgun' library
package main

import (
	"flag"
	"fmt"
	"github.com/DanielHeath/shotgun-go/webprocess"
	logger "log"
	"net/http"
	"net/url"
	"os"
)

var port int
var rawurl, checkCmd, buildCmd, runCmd string
var proxyUrl *url.URL

func init() {
	flag.IntVar(&port, "p", 8009, "Shorthand for --port")
	flag.IntVar(&port, "port", 8009, "The port for shotgun to listen on")
	flag.StringVar(&rawurl, "u", "", "Shorthand for --url")
	flag.StringVar(&rawurl, "url", "", "The url to proxy for")
	flag.StringVar(&checkCmd, "checkCmd", "", "Command to check if build is required. Command should exit 1 for a rebuild, 0 for no rebuild. (required)")
	flag.StringVar(&buildCmd, "buildCmd", "", "Command to build the executable (required)")
	flag.StringVar(&runCmd, "runCmd", "", "Command to run the executable (required)")
	flag.Parse()

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

func main() {
	log := logger.New(os.Stdout, "Shotgun: ", 0)
	log.Printf("Starting reverse proxy on http://localhost:%d for %s\n", port, proxyUrl.String())
	log.Println("Check command: " + checkCmd)
	log.Println("Build command: " + buildCmd)
	log.Println("Run command: " + runCmd)
	wp := webprocess.NewWebProcess(
		checkCmd,
		buildCmd,
		runCmd,
		proxyUrl,
		log,
	)
	http.HandleFunc("/", wp.ServeHTTP)
	panic(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}
