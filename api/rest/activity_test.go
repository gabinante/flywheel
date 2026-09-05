package rest

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gabinante/flywheel/internal/activity"
)

func TestActivityStreamThroughRouterAndReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := activity.New()
	hub.SourceReady(true)
	go hub.Run(ctx)
	server := httptest.NewServer(NewRouter(RouterConfig{ActivityHandler: &ActivityHandler{Hub: hub}, AuthMiddleware: LocalOperatorMiddleware("operator")}))
	defer server.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	connect := func() (*http.Response, *bufio.Reader) {
		t.Helper()
		req, _ := http.NewRequest("GET", server.URL+"/api/activity/events", nil)
		req.Header.Set("Last-Event-ID", "old-server-id")
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/event-stream" {
			t.Fatalf("response: %v", res)
		}
		return res, bufio.NewReader(res.Body)
	}
	read := func(reader *bufio.Reader) activity.Event {
		t.Helper()
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(line, "data: ") {
				var e activity.Event
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &e); err != nil {
					t.Fatal(err)
				}
				return e
			}
		}
	}
	res, reader := connect()
	defer res.Body.Close()
	if e := read(reader); !e.Ready || !e.Resync {
		t.Fatalf("initial: %+v", e)
	}
	hub.Publish(activity.Reviews)
	for e := read(reader); len(e.Topics) == 0; e = read(reader) {
	}
	res.Body.Close()
	hub.Publish(activity.Sessions)
	res2, reader2 := connect()
	defer res2.Body.Close()
	if e := read(reader2); !e.Ready || !e.Resync {
		t.Fatalf("reconnect: %+v", e)
	}
	request, _ := http.NewRequest("GET", server.URL+"/api/activity/events", nil)
	request.Header.Set("Origin", "https://attacker.test")
	denied, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer denied.Body.Close()
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("cross origin: %d", denied.StatusCode)
	}
}
