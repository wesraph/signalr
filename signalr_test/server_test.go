package signalr_test

import (
	"context"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/philippseith/signalr"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	npmInstall := exec.Command("npm", "install")
	stdout, err := npmInstall.StdoutPipe()
	if err != nil {
		os.Exit(120)
	}
	stderr, err := npmInstall.StderrPipe()
	if err != nil {
		os.Exit(121)
	}
	if err := npmInstall.Start(); err != nil {
		os.Exit(122)
	}
	outSlurp, _ := ioutil.ReadAll(stdout)
	errSlurp, _ := ioutil.ReadAll(stderr)
	err = npmInstall.Wait()
	if err != nil {
		fmt.Println(outSlurp)
		fmt.Println(errSlurp)
		os.Exit(123)
	}
	os.Exit(m.Run())
}

func TestServerWebSockets(t *testing.T) {
	testServer(t, "WebSockets")
}

func TestServerSSE(t *testing.T) {
	testServer(t, "ServerSentEvents")
}

func testServer(t *testing.T, connection string) {
	serverIsUp := make(chan struct{}, 1)
	quitServer := make(chan struct{}, 1)
	serverIsDown := make(chan struct{}, 1)
	go func() {
		runServer(t, serverIsUp, quitServer, []string{connection})
		serverIsDown <- struct{}{}
	}()
	<-serverIsUp
	runJest(t, quitServer)
	<-serverIsDown
}

func runJest(t *testing.T, quitServer chan struct{}) {
	defer func() { quitServer <- struct{}{} }()
	var jest = exec.Command(filepath.FromSlash("node_modules/.bin/jest"))
	stdout, err := jest.StdoutPipe()
	if err != nil {
		t.Error(err)
	}
	stderr, err := jest.StderrPipe()
	if err != nil {
		t.Error(err)
	}
	if err := jest.Start(); err != nil {
		t.Error(err)
	}
	outSlurp, _ := ioutil.ReadAll(stdout)
	errSlurp, _ := ioutil.ReadAll(stderr)
	err = jest.Wait()
	if err != nil {
		t.Error(err, fmt.Sprintf("\n%s\n%s", outSlurp, errSlurp))
	} else {
		// Strange: Jest reports test results to stderr
		t.Log(fmt.Sprintf("\n%s", errSlurp))
	}
}

func runServer(t *testing.T, serverIsUp chan struct{}, quitServer chan struct{}, transports []string) {
	// Install a handler to cancel the server
	doneQuit := make(chan struct{}, 1)
	sRServer, _ := signalr.NewServer(context.TODO(), signalr.SimpleHubFactory(&hub{}),
		signalr.KeepAliveInterval(2*time.Second),
		signalr.HttpTransports(transports...),
		signalr.Logger(log.NewLogfmtLogger(os.Stderr), true))
	router := sRServer.MapHub("/hub")

	server := &http.Server{
		Addr:         "127.0.0.1:5000",
		Handler:      router,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  10 * time.Second,
	}
	// wait for someone triggering quitServer
	go func() {
		<-quitServer
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		// If it does not Shutdown during 10s, try to end it by canceling the context
		defer cancel()
		server.SetKeepAlivesEnabled(false)
		_ = server.Shutdown(ctx)
		doneQuit <- struct{}{}
	}()
	// alternate method for quiting
	router.HandleFunc("/quit", func(http.ResponseWriter, *http.Request) {
		quitServer <- struct{}{}
	})
	t.Logf("Server %v is up", server.Addr)
	serverIsUp <- struct{}{}
	// Run the server
	_ = server.ListenAndServe()
	<-doneQuit
}

type hub struct {
	signalr.Hub
}

func (h *hub) Ping() string {
	return "Pong"
}

func (h *hub) TriumphantTriple(club string) []string {
	if strings.Contains(club, "FC Bayern") {
		return []string{"German Championship", "DFB Cup", "Champions League"}
	}
	return []string{}
}

type AlcoholicContent struct {
	Drink    string  `json:"drink"`
	Strength float32 `json:"strength"`
}

func (h *hub) AlcoholicContents() []AlcoholicContent {
	return []AlcoholicContent{
		{
			Drink:    "Brunello",
			Strength: 13.5,
		},
		{
			Drink:    "Beer",
			Strength: 4.9,
		},
		{
			Drink:    "Lagavulin Cask Strength",
			Strength: 56.2,
		},
	}
}

func (h *hub) AlcoholicContentMap() map[string]float64 {
	return map[string]float64{
		"Brunello":                13.5,
		"Beer":                    4.9,
		"Lagavulin Cask Strength": 56.2,
	}
}

func (h *hub) FiveDates() <-chan string {
	r := make(chan string)
	go func() {
		for i := 0; i < 5; i++ {
			r <- fmt.Sprint(time.Now().Nanosecond())
			time.Sleep(time.Millisecond * 100)
		}
		close(r)
	}()
	return r
}
