// A port of the ruby 'shotgun' library
package main

import (
	"flag"
	"fmt"
	"github.com/DanielHeath/shotgun-go/webprocess"
	"net/http"
	"net/url"
	"os"
)

var port int
var rawurl string
var proxyUrl *url.URL

func init() {
	flag.IntVar(&port, "p", 8009, "Shorthand for --port")
	flag.IntVar(&port, "port", 8009, "The port for shotgun to listen on")
	flag.StringVar(&rawurl, "u", "", "Shorthand for --url")
	flag.StringVar(&rawurl, "url", "", "The url to proxy for")
	flag.Parse()

	var err error
	proxyUrl, err = url.Parse(rawurl)
	if rawurl == "" || err != nil {
		fmt.Fprintf(os.Stderr, "Usage of %s: %s [options] command\n", os.Args[0], os.Args[0])
		flag.PrintDefaults()
		if rawurl != "" && err != nil {
			fmt.Fprintf(os.Stderr, "URL must be a valid url:", err)
		}
		os.Exit(-1)
	}
}

func main() {
	args := flag.Args()
	wp := webprocess.WebProcess{
		PackageName: args[0],
		TargetUrl:   proxyUrl,
	}
	http.HandleFunc("/", wp.ServeHTTP)
	panic(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}
