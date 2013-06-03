// Package transmission implements a library to talk to Transmission.
package transmission

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/StalkR/goircbot/lib/tls"
)

// A Statistics holds generic stats of Transmission.
type Statistics struct {
	DownloadSpeed, UploadSpeed                           int
	TorrentCount, ActiveTorrentCount, PausedTorrentcount int
}

func (s *Statistics) String() string {
	return fmt.Sprintf("%v KB/s DL, %v KB/s UL, %v torrents (%v active, %v paused)",
		s.DownloadSpeed/1024, s.UploadSpeed/1024, s.TorrentCount,
		s.ActiveTorrentCount, s.PausedTorrentcount)
}

// A Conn represents a connection to Transmission.
type Conn struct {
	url    string
	client http.Client
}

func timeoutDialer(d time.Duration) func(net, addr string) (net.Conn, error) {
	return func(netw, addr string) (net.Conn, error) {
		return net.DialTimeout(netw, addr, d)
	}
}

// New prepares a Transmission connection by returning a *Conn.
func New(rawurl string) (*Conn, error) {
	u, err := url.Parse(rawurl)
	if err != nil {
		return nil, err
	}
	return &Conn{
		url: rawurl,
		client: http.Client{
			Transport: &http.Transport{
				Dial:            timeoutDialer(5 * time.Second),
				TLSClientConfig: tls.Config(u.Host),
			},
		},
	}, nil
}

// sessionId asks Transmission for an RPC session ID.
func (c *Conn) sessionId() (string, error) {
	resp, err := c.client.Get(c.url + "/transmission/rpc")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	values, ok := resp.Header["X-Transmission-Session-Id"]
	if !ok || len(values) < 1 {
		return "", errors.New("transmission: sessionId not found")
	}
	return values[0], nil
}

// rpc sends an RPC request to Transmission with the right session ID.
func (c *Conn) rpc(request interface{}) ([]byte, error) {
	sessId, err := c.sessionId()
	if err != nil {
		return nil, err
	}
	js, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", c.url+"/transmission/rpc",
		bytes.NewBufferString(string(js)))
	if err != nil {
		return nil, err
	}
	req.Header.Add("X-Transmission-Session-Id", sessId)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

// Stats returns current statistics (speed, number of torrents, etc.).
func (c *Conn) Stats() (*Statistics, error) {
	b, err := c.rpc(map[string]string{"method": "session-stats"})
	if err != nil {
		return nil, err
	}
	var r sessionStats
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	if r.Result != "success" {
		return nil, fmt.Errorf("transmission: result: %s", r.Result)
	}
	return &r.Arguments, nil
}

type sessionStats struct {
	Arguments Statistics
	Result    string
}

// Add adds a torrent by URL and returns its name.
func (c *Conn) Add(url string) (string, error) {
	b, err := c.rpc(map[string]interface{}{
		"method": "torrent-add",
		"arguments": map[string]interface{}{
			"paused":   false,
			"filename": url,
		},
	})
	if err != nil {
		return "", err
	}
	var r torrentAdd
	if err := json.Unmarshal(b, &r); err != nil {
		return "", err
	}
	if r.Result != "success" {
		return "", fmt.Errorf("transmission: result: %s", r.Result)
	}
	if r.Arguments.TorrentAdded.Name == "" {
		return "", errors.New("transmission: empty result")
	}
	return r.Arguments.TorrentAdded.Name, nil
}

type torrentAdd struct {
	Arguments torrentAddedArguments
	Result    string
}

type torrentAddedArguments struct {
	TorrentAdded torrentAdded `json:"torrent-added"`
}

type torrentAdded struct {
	Id               int
	Name, HashString string
}