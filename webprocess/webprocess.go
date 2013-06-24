package webprocess

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type WebProcess struct {
	PackageName string
	TargetUrl   *url.URL
	command     *exec.Cmd
	output      bytes.Buffer
	m           sync.Mutex
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
			bytes, readErr := ioutil.ReadAll(&w.output)
			out := string(bytes)
			if readErr != nil {
				out = out + "\nFailed to read webprocess stdout: " + readErr.Error()
			}
			fmt.Println(w.running())
			fmt.Println(w.command)
			http.Error(rw, err.Error()+"\nBEGIN Webprocess stdout:\n"+out+"\nEND stdout", http.StatusInternalServerError)
			//return
		}
	}
	proxy := httputil.NewSingleHostReverseProxy(w.TargetUrl)
	proxy.ServeHTTP(responseWrapper{rw, w}, r)
}

func (w *WebProcess) reload() (err error) {
	w.stop()
	err = w.rebuild()
	if err != nil {
		return
	}
	err = w.start()
	if err != nil {
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

func (w *WebProcess) rebuild() error {
	buildout, err := exec.Command("go", "build", "-o", "/tmp/shotgunner", w.PackageName).CombinedOutput()
	if err != nil {
		return errors.New(err.Error() + string(buildout))
	}
	return nil
}

func (w *WebProcess) start() error {
	if w.running() {
		return errors.New("Can't start, already running.")
	}
	w.command = exec.Command("/tmp/shotgunner")
	w.command.Stdout = &w.output
	w.command.Stderr = &w.output
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
	ticker := time.NewTicker(time.Millisecond * 50)
	defer ticker.Stop()

	for _ = range ticker.C {
		if !w.running() {
			return errors.New("Process not running")
		}
		if w.up() {
			return nil
		}

		fmt.Print(".")
		ticks++
		if ticks > 70 {
			fmt.Print("Giving up")
			return errors.New("Process did not listen after waiting 70*50ms")
		}
	}
	panic("How did we get here? That channel should block forever...")
	return nil
}

func (w *WebProcess) rebuildRequired() bool {
	uptodate := exec.Command(
		"bash",
		"-c",
		"[ ! -e /tmp/shotgunner ] || find src -newer /tmp/shotgunner | grep -v .",
	)
	uptodate.CombinedOutput() // Start and wait for completion
	return !uptodate.ProcessState.Success()
}
