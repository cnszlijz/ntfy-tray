// ntfy-tray: Windows tray app that subscribes to ntfy.sh topics and shows
// toast notifications.
package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/getlantern/systray"
	"gopkg.in/toast.v1"
)

const (
	appID            = "ntfy.sh Notifier"
	watchdogInterval = 90 * time.Second // server keepalives arrive every ~45s
	maxBackoff       = 30 * time.Second
)

//go:embed icon.ico
var iconData []byte

var (
	server = flag.String("server", "https://ntfy.sh", "ntfy server URL")
	topics = flag.String("topics", "", "comma-separated ntfy topics to subscribe (required)")
	token  = flag.String("token", "", "ntfy access token (tk_...) for protected topics")
)

type ntfyMessage struct {
	ID       string   `json:"id"`
	Time     int64    `json:"time"`
	Event    string   `json:"event"` // open, keepalive, message, poll_request
	Topic    string   `json:"topic"`
	Title    string   `json:"title"`
	Message  string   `json:"message"`
	Priority int      `json:"priority"`
	Tags     []string `json:"tags"`
	Click    string   `json:"click"`
}

func main() {
	flag.Parse()
	if *topics == "" {
		log.Fatal("-topics is required, e.g. -topics=mytopic,alerts")
	}
	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetIcon(iconData)
	systray.SetTitle("")
	systray.SetTooltip("ntfy notifier — " + *topics)

	mOpen := systray.AddMenuItem("Open ntfy", "Open topic in browser")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Quit the notifier")

	ctx, cancel := context.WithCancel(context.Background())
	go listenLoop(ctx)

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openBrowser(fmt.Sprintf("%s/%s", strings.TrimRight(*server, "/"), firstTopic()))
			case <-mQuit.ClickedCh:
				cancel()
				systray.Quit()
				return
			}
		}
	}()
}

func onExit() {}

func openBrowser(url string) {
	// No console window: -H=windowsgui build + rundll32 handles the open.
	if err := exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start(); err != nil {
		log.Printf("open browser failed: %v", err)
	}
}

func firstTopic() string {
	return strings.SplitN(*topics, ",", 2)[0]
}

// listenLoop maintains the stream forever, reconnecting with backoff.
func listenLoop(ctx context.Context) {
	backoff := time.Second
	for {
		err := stream(ctx)
		if ctx.Err() != nil {
			return
		}
		log.Printf("stream ended: %v; reconnecting in %v", err, backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// stream opens one long-lived NDJSON subscription until error or cancel.
func stream(ctx context.Context) error {
	url := fmt.Sprintf("%s/%s/json?since=all", strings.TrimRight(*server, "/"), *topics)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if *token != "" {
		req.Header.Set("Authorization", "Bearer "+*token)
	}

	// No client timeout: this connection is supposed to live forever.
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ntfy returned %s (check topic/token)", resp.Status)
	}

	lines := make(chan []byte)
	scanErr := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			b := append([]byte(nil), sc.Bytes()...)
			lines <- b
		}
		scanErr <- sc.Err()
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-scanErr:
			if err == nil {
				err = fmt.Errorf("server closed connection")
			}
			return err
		case b := <-lines:
			var msg ntfyMessage
			if err := json.Unmarshal(b, &msg); err != nil {
				continue
			}
			if msg.Event == "message" {
				notify(msg)
			}
			// "open"/"keepalive" frames only serve to reset the watchdog below.
		case <-time.After(watchdogInterval):
			return fmt.Errorf("no keepalive within %v, reconnecting", watchdogInterval)
		}
	}
}

// notify shows a Windows toast for one ntfy message.
func notify(msg ntfyMessage) {
	title := msg.Title
	if title == "" {
		title = msg.Topic
	}
	click := msg.Click
	if click == "" {
		click = fmt.Sprintf("%s/%s", strings.TrimRight(*server, "/"), msg.Topic)
	}

	n := toast.Notification{
		AppID:               appID,
		Title:               title,
		Message:             msg.Message,
		ActivationArguments: click,
		Actions: []toast.Action{
			{Type: "protocol", Label: "Open", Arguments: click},
		},
	}
	if msg.Priority >= 4 { // high/urgent
		n.Audio = toast.LoopingAlarm
	}
	if err := n.Push(); err != nil {
		log.Printf("toast failed: %v", err)
	}
}
