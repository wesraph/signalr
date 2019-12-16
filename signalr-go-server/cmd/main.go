package main

import (
	"../pkg/signalr"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type chat struct {
	signalr.Hub
}

func (c *chat) OnConnected(connectionID string) {
	fmt.Printf("%s connected\n", connectionID)
	c.Groups().AddToGroup("group", connectionID)
}

func (c *chat) OnDisconnected(connectionID string) {
	fmt.Printf("%s disconnected\n", connectionID)
	c.Groups().RemoveFromGroup("group", connectionID)
}

func (c *chat) Send(message string) {
	c.Clients().Group("group").Send("send", message)
}

func (c *chat) Echo(message string) {
	c.Clients().Caller().Send("send", message)
}

func (c *chat) Panic() {
	panic("Don't panic!")
}

func (c *chat) RequestAsync(message string) <-chan map[string]string {
	r := make(chan map[string]string)
	go func() {
		defer close(r)
		time.Sleep(5 * time.Second)
		m := make(map[string]string)
		m["ToUpper"] = strings.ToUpper(message)
		m["ToLower"] = strings.ToLower(message)
		m["len"] = fmt.Sprint(len(message))
		r <- m
	}()
	return r
}

func (c *chat) RequestTuple(message string) (string, string, int) {
	return strings.ToUpper(message), strings.ToLower(message), len(message)
}

func (c *chat) DateStream() <-chan string {
	r := make(chan string)
	go func() {
		defer close(r)
		for i := 0;  i < 5; i++ {
			r <- fmt.Sprint(time.Now().Clock())
			time.Sleep(time.Second)
		}
	}()
	return r
}

func main() {
	router := http.NewServeMux()
	router.Handle("/", http.FileServer(http.Dir("../public")))

	signalr.MapHub(router, "/chat", &chat{})

	if err := http.ListenAndServe("localhost:8087", router); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
