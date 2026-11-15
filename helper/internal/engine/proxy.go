package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"splitlane/helper/internal/connector"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

// The proxy connector (spec of stage 3 part 1, sections 3-5): the servers and subscriptions of
// servers.json, the commands proxy.add, proxy.refresh, proxy.remove and proxy.select, and what the state
// shows of them. The secrets (the texts of the servers, the addresses of the subscriptions) stay in
// servers.json: the state, state.json, the answers and the log carry ids, names, protocols, sources, hosts
// and codes only.

// noFetch is the Fetcher of an engine that was given none: every download is refused.
type noFetch struct{}

func (noFetch) Get(context.Context, string) ([]byte, error) {
	return nil, &connector.FetchError{Code: connector.CodeFetchFailed, Detail: "no downloader"}
}

// connectorOf is the connector of a saved state: proxy, or tailscale (an old state.json has none).
func connectorOf(s store.State) string {
	if s.Connector == protocol.ConnectorProxy {
		return protocol.ConnectorProxy
	}
	return protocol.ConnectorTailscale
}

// subscriptionID is the id of the subscription at an address: "sub-" and the first 8 bytes of the SHA-256
// of the address in hex. The same address is the same subscription; the id gives the address (and its
// token) away no more than connector.ServerID gives a link away.
func subscriptionID(address string) string {
	sum := sha256.Sum256([]byte(address))
	return "sub-" + hex.EncodeToString(sum[:8])
}

// cloneServers is a copy of a servers.json that shares nothing with it.
func cloneServers(s store.Servers) store.Servers {
	s.Servers = append([]store.StoredServer{}, s.Servers...)
	s.Subscriptions = append([]store.StoredSubscription{}, s.Subscriptions...)
	return s
}

// serverLocked is the server with the id, nil when there is none (or id is ""); e.mu must be held.
func (e *Engine) serverLocked(id string) *store.StoredServer {
	if id == "" {
		return nil
	}
	for i := range e.servers.Servers {
		if e.servers.Servers[i].ID == id {
			return &e.servers.Servers[i]
		}
	}
	return nil
}

// subscriptionLocked is the subscription with the id, nil when there is none; e.mu must be held.
func (e *Engine) subscriptionLocked(id string) *store.StoredSubscription {
	for i := range e.servers.Subscriptions {
		if e.servers.Subscriptions[i].ID == id {
			return &e.servers.Subscriptions[i]
		}
	}
	return nil
}

// syncProxy copies the connector and the servers into the published state, nothing secret; e.mu must be
// held. A selected id that no server has (servers.json was set aside or cannot be read now) shows as none.
func (e *Engine) syncProxy() {
	e.st.Connector = connectorOf(e.saved)
	p := protocol.Proxy{
		Servers:       make([]protocol.ProxyServer, 0, len(e.servers.Servers)),
		Subscriptions: make([]protocol.Subscription, 0, len(e.servers.Subscriptions)),
	}
	for _, s := range e.servers.Servers {
		p.Servers = append(p.Servers, protocol.ProxyServer{ID: s.ID, Name: s.Name, Protocol: s.Protocol, Source: s.Source})
	}
	if e.serverLocked(e.saved.SelectedServer) != nil {
		id := e.saved.SelectedServer
		p.Selected = &id
	}
	for _, s := range e.servers.Subscriptions {
		sub := protocol.Subscription{ID: s.ID, Name: s.Name, UpdatedAt: s.UpdatedAt, Count: s.Count}
		if s.Error != "" {
			code := s.Error
			sub.Error = &code
		}
		p.Subscriptions = append(p.Subscriptions, sub)
	}
	e.st.Proxy = p
}

// serversReadyLocked reads servers.json again when it could not be read before (load): until it reads, the
// proxy commands are apply_failed and nothing is written over it. e.mu must be held.
func (e *Engine) serversReadyLocked() *protocol.Error {
	if e.serversErr == nil {
		return nil
	}
	s, err := e.d.Store.LoadServers()
	switch {
	case errors.Is(err, store.ErrIO):
		e.serversErr = err
		e.d.Log("servers.json still cannot be read: %v", err)
		return protocol.Errorf(protocol.CodeApplyFailed, "servers.json cannot be read now")
	case err != nil:
		e.d.Log("servers.json: %v", err)
	}
	e.d.Log("servers.json can be read now")
	e.servers, e.serversErr = s, nil
	return nil
}

// saveServersLocked writes next as servers.json and takes it as the servers of the engine; a write that
// fails changes nothing (apply_failed), and more servers or subscriptions than the limits allow is
// too_many. e.mu must be held.
func (e *Engine) saveServersLocked(next store.Servers) *protocol.Error {
	err := e.d.Store.SaveServers(next)
	switch {
	case errors.Is(err, store.ErrTooMany):
		return protocol.Errorf(protocol.CodeTooMany, "%v", err)
	case err != nil:
		e.d.Log("servers.json not saved: %v", err)
		return protocol.Errorf(protocol.CodeApplyFailed, "servers.json not saved")
	}
	e.servers = next
	return nil
}

// proxyPhaseLocked is the phase check of a proxy command that changes the servers. One that touches what
// routing runs on or is about to run on (the selected server while the connector is proxy) is busy in
// starting and stopping and while an operation that may enable is in place, and wrong_phase while routing
// is live: the core runs on its config until routing is off (Review Focus 3). Any other change is fine in
// every phase: the core reads nothing of servers.json while it runs. After helper.shutdown all are busy.
// e.mu must be held.
func (e *Engine) proxyPhaseLocked(touchesSelected bool) *protocol.Error {
	phase := e.st.Phase
	switch {
	case e.exiting:
		return busy()
	case !touchesSelected || connectorOf(e.saved) != protocol.ConnectorProxy:
		return nil
	case live(phase):
		return wrongPhase(phase)
	case phase == protocol.PhaseStarting || phase == protocol.PhaseStopping || e.op != nil:
		return busy()
	}
	return nil
}

// selectFirstLocked: the first server added becomes the selected one when none is (stage 3 part 1,
// section 4). In off and error, with no operation in place, the connector becomes proxy with it, so that
// routing.enable uses it; while routing is live, starting or stopping the connector stays: only
// proxy.select reconnects. e.mu must be held.
func (e *Engine) selectFirstLocked(added []string) {
	if len(added) == 0 || e.serverLocked(e.saved.SelectedServer) != nil {
		return
	}
	e.saved.SelectedServer = added[0]
	if (e.st.Phase == protocol.PhaseOff || e.st.Phase == protocol.PhaseError) && e.op == nil {
		e.saved.Connector = protocol.ConnectorProxy
	}
	e.save()
}

// dropGoneSelectionLocked clears the selection when no server has its id any more (a refresh or a removal
// took it); the connector stays, and an enable is then no_server. e.mu must be held.
func (e *Engine) dropGoneSelectionLocked() {
	if e.saved.SelectedServer != "" && e.serverLocked(e.saved.SelectedServer) == nil {
		e.d.Log("the selected server is gone: no server is selected")
		e.saved.SelectedServer = ""
		e.save()
	}
}

func storedServer(s connector.Server, source string) store.StoredServer {
	return store.StoredServer{ID: s.ID, Name: s.Name, Protocol: s.Protocol, Source: source, Text: s.Text}
}

// addResult is the answer of proxy.add and proxy.refresh: at most MaxRejected rejected lines, the others
// counted.
func addResult(added []string, rejected []connector.Rejection) protocol.ProxyAddResult {
	r := protocol.ProxyAddResult{Added: append([]string{}, added...), Rejected: []protocol.ProxyRejection{}}
	for i, x := range rejected {
		if i == protocol.MaxRejected {
			r.RejectedMore = len(rejected) - i
			break
		}
		r.Rejected = append(r.Rejected, protocol.ProxyRejection{Line: x.Line, Code: x.Code})
	}
	return r
}

// codes lists the codes of rejections once each, for the log.
func codes(rejected []connector.Rejection) string {
	var out []string
	for _, r := range rejected {
		if !slices.Contains(out, r.Code) {
			out = append(out, r.Code)
		}
	}
	if len(out) == 0 {
		return "-"
	}
	return strings.Join(out, ",")
}

// cmdProxyAdd is proxy.add (stage 3 part 1, section 4): links, the text of a .conf, or a subscription
// address (subscribe). The servers that are not there yet go into servers.json; one that is there (the
// same id) stays as it is. An add that would take servers.json over its limit adds nothing: the rejection
// too_many of the whole text. The first server added becomes the selected one when none is
// (selectFirstLocked). Every phase but after helper.shutdown: nothing of it changes the running core.
func (e *Engine) cmdProxyAdd(ctx context.Context, params json.RawMessage) (any, *protocol.Error) {
	var p protocol.ProxyAddParams
	if err := protocol.DecodeParams(params, &p); err != nil {
		return nil, err
	}
	if err := protocol.ValidateProxyAdd(p); err != nil {
		return nil, err
	}
	servers, rejected, address := connector.ParseText(p.Text, p.Name)
	if address != "" {
		return e.subscribe(ctx, subscriptionID(address), address, true)
	}
	source := protocol.SourceManual
	if p.Name != "" {
		source = protocol.SourceFile
	}
	e.queue.Lock()
	defer e.queue.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.proxyPhaseLocked(false); err != nil {
		return nil, err
	}
	if err := e.serversReadyLocked(); err != nil {
		return nil, err
	}
	next := cloneServers(e.servers)
	have := map[string]bool{}
	for _, s := range next.Servers {
		have[s.ID] = true
	}
	var added []string
	for _, s := range servers {
		added = append(added, s.ID)
		if !have[s.ID] {
			have[s.ID] = true
			next.Servers = append(next.Servers, storedServer(s, source))
		}
	}
	if len(next.Servers) > store.MaxServers {
		e.d.Log("proxy.add: %d servers would be more than %d: nothing added", len(next.Servers), store.MaxServers)
		return addResult(nil, append([]connector.Rejection{{Code: protocol.CodeTooMany}}, rejected...)), nil
	}
	fresh := len(next.Servers) - len(e.servers.Servers)
	if fresh > 0 {
		if err := e.saveServersLocked(next); err != nil {
			return nil, err
		}
	}
	e.selectFirstLocked(added)
	e.publish()
	e.d.Log("proxy.add: %d servers in the text, %d new; %d lines rejected (%s)", len(servers), fresh, len(rejected), codes(rejected))
	return addResult(added, rejected), nil
}

// cmdProxyRefresh is proxy.refresh (stage 3 part 1, section 4): the subscription is downloaded again
// (subscribe).
func (e *Engine) cmdProxyRefresh(ctx context.Context, params json.RawMessage) (any, *protocol.Error) {
	var p protocol.ProxyIDParams
	if err := protocol.DecodeParams(params, &p); err != nil {
		return nil, err
	}
	if err := protocol.ValidateProxyID(p.ID); err != nil {
		return nil, err
	}
	e.mu.Lock()
	err := e.serversReadyLocked()
	address := ""
	if sub := e.subscriptionLocked(p.ID); sub != nil {
		address = sub.URL
	}
	e.mu.Unlock()
	switch {
	case err != nil:
		return nil, err
	case address == "":
		return nil, protocol.Errorf(protocol.CodeUnknownServer, "no subscription has this id")
	}
	return e.subscribe(ctx, p.ID, address, false)
}

// subscribePhaseLocked is the check of subscribe before and after the download: not after
// helper.shutdown, servers.json read, the subscription there for a refresh, room for a new one (at most
// MaxSubscriptions), and the phase: a subscription whose server routing runs on is not refreshed while
// routing is live (proxyPhaseLocked). e.mu must be held.
func (e *Engine) subscribePhaseLocked(id string, add bool) *protocol.Error {
	if e.exiting {
		return busy()
	}
	if err := e.serversReadyLocked(); err != nil {
		return err
	}
	old := e.subscriptionLocked(id)
	switch {
	case old == nil && !add:
		return protocol.Errorf(protocol.CodeUnknownServer, "no subscription has this id")
	case old == nil && len(e.servers.Subscriptions) >= store.MaxSubscriptions:
		return protocol.Errorf(protocol.CodeTooMany, "at most %d subscriptions", store.MaxSubscriptions)
	}
	sel := e.serverLocked(e.saved.SelectedServer)
	return e.proxyPhaseLocked(old != nil && sel != nil && sel.Source == id)
}

// fetchCode is the code of a failed download: the FetchError's, else fetch_failed.
func fetchCode(err error) string {
	var fe *connector.FetchError
	if errors.As(err, &fe) && fe.Code != "" {
		return fe.Code
	}
	return connector.CodeFetchFailed
}

// hostOf is the name a subscription is shown by: the host of its address (no path, no query, no user:
// those may hold its token).
func hostOf(address string) string {
	if u, err := url.Parse(address); err == nil && u.Hostname() != "" {
		return strings.ToLower(u.Hostname())
	}
	return "subscription"
}

// subscribe is proxy.add of a subscription address (add: a new subscription, or the same address again,
// which refreshes it) and proxy.refresh (stage 3 part 1, sections 3, 4). The checks come first, so that a
// refusal costs no download; the download (connector.Fetcher: https only, no local address, 1 MB, 20 s)
// runs outside the queue of the state-changing commands, and the queue is taken to apply what it gave,
// after the checks once more. The servers of the subscription are replaced by those it gives now (one
// that another source holds stays with it); the selected server stays selected when a server with its
// id is still there, else the selection goes. A download that fails, a subscription without a server and
// one that would take servers.json over its limit change no server: an existing subscription keeps the
// code in its error, which the state shows; proxy.add answers with the rejection of the whole text,
// proxy.refresh with the code as its error. The address is never in an answer, the state or the log.
func (e *Engine) subscribe(ctx context.Context, id, address string, add bool) (any, *protocol.Error) {
	e.mu.Lock()
	err := e.subscribePhaseLocked(id, add)
	e.mu.Unlock()
	if err != nil {
		return refusedSubscription(add, err)
	}
	body, ferr := e.d.Fetch.Get(ctx, address)
	var servers []connector.Server
	var rejected []connector.Rejection
	if ferr == nil {
		servers, rejected = connector.ParseSubscription(body)
	}
	e.queue.Lock()
	defer e.queue.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.subscribePhaseLocked(id, add); err != nil { // the phase or the servers changed during the download
		return refusedSubscription(add, err)
	}
	code := ""
	switch {
	case ferr != nil:
		code = fetchCode(ferr)
		e.d.Log("subscription %s not downloaded: %s", id, connector.Mask(ferr.Error()))
	case len(servers) == 0:
		code = protocol.CodeEmptySubscription
		if len(rejected) > 0 {
			code = rejected[0].Code
		}
		e.d.Log("subscription %s gave no server: %d lines rejected (%s)", id, len(rejected), codes(rejected))
	}
	next := cloneServers(e.servers)
	var added []string
	count := 0
	if code == "" {
		kept := make([]store.StoredServer, 0, len(next.Servers)+len(servers))
		have := map[string]bool{}
		for _, s := range next.Servers {
			if s.Source != id {
				kept, have[s.ID] = append(kept, s), true
			}
		}
		for _, s := range servers {
			added = append(added, s.ID)
			if !have[s.ID] {
				have[s.ID] = true
				kept = append(kept, storedServer(s, id))
				count++
			}
		}
		next.Servers = kept
		if len(kept) > store.MaxServers {
			code = protocol.CodeTooMany
			e.d.Log("subscription %s: %d servers would be more than %d: not applied", id, len(kept), store.MaxServers)
		}
	}
	if code != "" {
		e.subscriptionFailedLocked(id, code)
		if !add {
			return nil, protocol.Errorf(code, "subscription not refreshed: %s", code)
		}
		switch {
		case code == protocol.CodeTooMany:
			rejected = append([]connector.Rejection{{Code: code}}, rejected...)
		case ferr != nil || len(rejected) == 0:
			rejected = []connector.Rejection{{Code: code}}
		}
		return addResult(nil, rejected), nil
	}
	sub := store.StoredSubscription{ID: id, Name: hostOf(address), URL: address,
		UpdatedAt: e.d.Clock.Now().UTC().Format(time.RFC3339), Count: count}
	if i := slices.IndexFunc(next.Subscriptions, func(s store.StoredSubscription) bool { return s.ID == id }); i >= 0 {
		next.Subscriptions[i] = sub
	} else {
		next.Subscriptions = append(next.Subscriptions, sub)
	}
	if err := e.saveServersLocked(next); err != nil {
		return nil, err
	}
	e.dropGoneSelectionLocked()
	if add {
		e.selectFirstLocked(added)
	}
	e.publish()
	e.d.Log("subscription %s: %d servers, %d of them its own; %d lines rejected (%s)", id, len(servers), count, len(rejected), codes(rejected))
	return addResult(added, rejected), nil
}

// subscriptionFailedLocked keeps the code of a failed update in the subscription, when it is there, for
// the window to show; its servers stay. e.mu must be held.
func (e *Engine) subscriptionFailedLocked(id, code string) {
	if e.subscriptionLocked(id) == nil {
		return
	}
	next := cloneServers(e.servers)
	for i := range next.Subscriptions {
		if next.Subscriptions[i].ID == id {
			next.Subscriptions[i].Error = code
		}
	}
	if err := e.saveServersLocked(next); err != nil {
		e.d.Log("subscription %s: its error not saved: %s", id, err.Message)
		return
	}
	e.publish()
}

// refusedSubscription is the answer to a subscription refused before it was applied: for proxy.add the
// limit of subscriptions is the rejection too_many of the whole text, as the limit of servers is.
func refusedSubscription(add bool, err *protocol.Error) (any, *protocol.Error) {
	if add && err.Code == protocol.CodeTooMany {
		return addResult(nil, []connector.Rejection{{Code: protocol.CodeTooMany}}), nil
	}
	return nil, err
}

// cmdProxyRemove is proxy.remove (stage 3 part 1, section 4): a server, or a subscription with all its
// servers. The selected server goes from the selection; while routing is live with it, neither it nor its
// subscription is removed (wrong_phase).
func (e *Engine) cmdProxyRemove(params json.RawMessage) *protocol.Error {
	var p protocol.ProxyIDParams
	if err := protocol.DecodeParams(params, &p); err != nil {
		return err
	}
	if err := protocol.ValidateProxyID(p.ID); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.exiting {
		return busy()
	}
	if err := e.serversReadyLocked(); err != nil {
		return err
	}
	sel := e.serverLocked(e.saved.SelectedServer)
	next := cloneServers(e.servers)
	touches := false
	if protocol.IsSubscriptionID(p.ID) {
		if e.subscriptionLocked(p.ID) == nil {
			return protocol.Errorf(protocol.CodeUnknownServer, "no subscription has this id")
		}
		touches = sel != nil && sel.Source == p.ID
		next.Servers = slices.DeleteFunc(next.Servers, func(s store.StoredServer) bool { return s.Source == p.ID })
		next.Subscriptions = slices.DeleteFunc(next.Subscriptions, func(s store.StoredSubscription) bool { return s.ID == p.ID })
	} else {
		st := e.serverLocked(p.ID)
		if st == nil {
			return protocol.Errorf(protocol.CodeUnknownServer, "no server has this id")
		}
		touches = sel != nil && sel.ID == p.ID
		next.Servers = slices.DeleteFunc(next.Servers, func(s store.StoredServer) bool { return s.ID == p.ID })
		for i := range next.Subscriptions {
			if next.Subscriptions[i].ID == st.Source && next.Subscriptions[i].Count > 0 {
				next.Subscriptions[i].Count--
			}
		}
	}
	if err := e.proxyPhaseLocked(touches); err != nil {
		return err
	}
	if err := e.saveServersLocked(next); err != nil {
		return err
	}
	e.dropGoneSelectionLocked()
	e.publish()
	e.d.Log("proxy.remove: %s removed; %d servers, %d subscriptions", p.ID, len(next.Servers), len(next.Subscriptions))
	return nil
}

// cmdProxySelect is proxy.select (stage 3 part 1, section 4): the server becomes the selected one and the
// connector proxy; while routing is live it disables and enables again, as tailscale.setExitNode does
// (reconnectLocked). The same server with the connector already proxy changes nothing.
func (e *Engine) cmdProxySelect(params json.RawMessage) *protocol.Error {
	var p protocol.ProxyIDParams
	if err := protocol.DecodeParams(params, &p); err != nil {
		return err
	}
	if err := protocol.ValidateProxyID(p.ID); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.reconnectPhaseLocked(); err != nil {
		return err
	}
	if err := e.serversReadyLocked(); err != nil {
		return err
	}
	if e.serverLocked(p.ID) == nil {
		return protocol.Errorf(protocol.CodeUnknownServer, "no server has this id")
	}
	if e.saved.SelectedServer == p.ID && connectorOf(e.saved) == protocol.ConnectorProxy {
		return nil
	}
	e.saved.SelectedServer, e.saved.Connector = p.ID, protocol.ConnectorProxy
	e.save()
	if !live(e.st.Phase) {
		e.publish()
		return nil
	}
	e.reconnectLocked()
	return nil
}
