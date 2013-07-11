package webprocess

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	logger "log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type WebProcess struct {
	CheckCmd  string
	BuildCmd  string
	RunCmd    string
	TargetUrl *url.URL
	command   *exec.Cmd
	output    bytes.Buffer
	m         sync.Mutex
	Log       *logger.Logger
}

func NewWebProcess(checkCmd, buildCmd, runCmd string, targeturl *url.URL, log *logger.Logger) *WebProcess {
	return &WebProcess{
		CheckCmd:  checkCmd,
		BuildCmd:  buildCmd,
		RunCmd:    runCmd,
		TargetUrl: targeturl,
		Log:       log,
	}
}

type responseWrapper struct {
	http.ResponseWriter
	*WebProcess
}

func (r responseWrapper) WriteHeader(code int) {
	r.ResponseWriter.WriteHeader(code)
	if code == http.StatusInternalServerError {
		io.Copy(r.ResponseWriter, &r.WebProcess.output)
	}
}
func (w *WebProcess) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	w.m.Lock()
	defer w.m.Unlock()

	if w.rebuildRequired() || (w.command == nil) {
		err := w.reload()
		if err != nil {
			bytes, _ := ioutil.ReadAll(&w.output)
			output := string(bytes)
			http.Error(rw, err.Error()+"\n\n"+output, http.StatusInternalServerError)
		}
	}
	proxy := httputil.NewSingleHostReverseProxy(w.TargetUrl)
	proxy.ServeHTTP(responseWrapper{rw, w}, r)
}

func (w *WebProcess) reload() (err error) {
	w.Log.Println("Reloading...")
	w.stop()
	buildout, err := w.rebuild()
	if err != nil {
		w.Log.Println(err)
		w.Log.Println(buildout)
		return
	}
	err = w.start()
	if err != nil {
		w.Log.Println(err)
		return
	}
	err = w.waitUntilUp()
	return
}

func (w *WebProcess) stop() {
	if w.command != nil {
		if w.command.Process != nil {
			w.command.Process.Signal(syscall.SIGTERM)
		}
		w.command.Wait()
		w.clearCmd()
	}
}
func (w *WebProcess) clearCmd() {
	w.command = nil
	w.output = bytes.Buffer{}
}

func (w *WebProcess) rebuild() ([]byte, error) {
	w.Log.Println("Build: " + w.BuildCmd)
	return exec.Command("bash", "-c", w.BuildCmd).CombinedOutput()
}

func (w *WebProcess) start() error {
	w.Log.Println("Start: " + w.RunCmd)
	if w.running() {
		return errors.New("Can't start, already running.")
	}

	w.command = exec.Command("bash", "-c", w.RunCmd)
	w.command.Stdout = io.MultiWriter(&w.output, os.Stdout)
	w.command.Stderr = io.MultiWriter(&w.output, os.Stderr)

	return w.command.Start()
}

func (w *WebProcess) running() bool {
	return ((w.command != nil) &&
		(w.command.Process != nil) &&
		((w.command.ProcessState == nil) || !w.command.ProcessState.Exited()))
}

func (w *WebProcess) up() bool {
	_, err := http.Head(w.TargetUrl.String())
	return err == nil
}

func (w *WebProcess) waitUntilUp() error {
	var ticks int
	if !w.running() {
		return errors.New("Not running.")
	}
	if w.up() {
		return nil
	}
	w.Log.Println("Waiting for process...")
	ticker := time.NewTicker(time.Millisecond * 200)
	defer ticker.Stop()

	for _ = range ticker.C {
		if !w.running() {
			return errors.New("Process not running")
		}
		if w.up() {
			return nil
		}
		w.Log.Print(".")
		ticks++
		if ticks > 20 {
			w.Log.Print("Giving up")
			return errors.New("Process did not listen after waiting 20*200ms")
		}
	}
	panic("How did we get here? That channel should block forever...")
	return nil
}

func (w *WebProcess) rebuildRequired() bool {
	w.Log.Println("Check: " + w.CheckCmd)
	uptodate := exec.Command("bash", "-c", w.CheckCmd).Run()

	return uptodate != nil
}
