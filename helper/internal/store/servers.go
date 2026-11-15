package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Limits of servers.json, enforced on save (spec 3).
const (
	MaxServers       = 500
	MaxSubscriptions = 20
)

// ErrTooMany marks a save refused because servers.json would hold more than the limits allow.
var ErrTooMany = errors.New("store: too many servers or subscriptions")

// StoredServer is a server of servers.json: Text is the normalized source (a link or a .conf), a secret.
type StoredServer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"`
	Text   string `json:"text"`
}

// StoredSubscription is a subscription of servers.json; its URL is a secret.
type StoredSubscription struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updatedAt"`
	Count     int    `json:"count"`
	Error     string `json:"error"`
}

// Servers is servers.json. The file holds secrets and is never part of the diagnostics archive.
type Servers struct {
	Version       int                  `json:"version"`
	Servers       []StoredServer       `json:"servers"`
	Subscriptions []StoredSubscription `json:"subscriptions"`
}

// EmptyServers is servers.json of a fresh install.
func EmptyServers() Servers {
	return Servers{Version: 1, Servers: []StoredServer{}, Subscriptions: []StoredSubscription{}}
}

func (s *Servers) fill() {
	if s.Version == 0 {
		s.Version = 1
	}
	if s.Servers == nil {
		s.Servers = []StoredServer{}
	}
	if s.Subscriptions == nil {
		s.Subscriptions = []StoredSubscription{}
	}
}

// LoadServers reads servers.json as LoadState reads state.json: missing gives an empty set; an
// unreadable file gives an error that wraps ErrIO and stays; unreadable content is set aside as
// servers.json.bad (or the first free .bad.<n>) and gives an empty set with an error.
func (d Dir) LoadServers() (Servers, error) {
	s := EmptyServers()
	b, err := readFile(d.servers())
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("%w: %w", ErrIO, err)
	}
	if cause := json.Unmarshal(b, &s); cause != nil {
		return EmptyServers(), setAside(d.servers(), cause)
	}
	s.fill()
	return s, nil
}

// SaveServers writes servers.json atomically; more than MaxServers servers or MaxSubscriptions
// subscriptions is refused (ErrTooMany) and nothing is written.
func (d Dir) SaveServers(s Servers) error {
	if len(s.Servers) > MaxServers {
		return fmt.Errorf("%w: %d servers, at most %d", ErrTooMany, len(s.Servers), MaxServers)
	}
	if len(s.Subscriptions) > MaxSubscriptions {
		return fmt.Errorf("%w: %d subscriptions, at most %d", ErrTooMany, len(s.Subscriptions), MaxSubscriptions)
	}
	s.fill()
	return d.writeJSON(d.servers(), s)
}
