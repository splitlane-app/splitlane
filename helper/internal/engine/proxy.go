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

// laneLocked is the connector of the VPN lane for a core built now from the saved state (stage 3 part 1,
// section 5): the embedded Tailscale node with the selected exit node, or the selected server parsed again
// from its text in servers.json (the parser is the one source of truth). For the proxy connector a server
// that is not selected, not in servers.json, does not parse, or a servers.json that cannot be read is
// no_server; the log says which, never with the text. e.mu must be held.
func (e *Engine) laneLocked() (Lane, *protocol.Reason) {
	if connectorOf(e.saved) != protocol.ConnectorProxy {
		return Lane{Tailscale: &TailscaleLane{Hostname: e.hostname(), ExitNode: e.saved.SelectedExitNode}}, nil
	}
	if err := e.serversReadyLocked(); err != nil {
		e.d.Log("no server for the VPN lane: servers.json cannot be read")
		return Lane{}, reason(protocol.ReasonNoServer)
	}
	st := e.serverLocked(e.saved.SelectedServer)
	if st == nil {
		e.d.Log("no server for the VPN lane: none is selected")
		return Lane{}, reason(protocol.ReasonNoServer)
	}
	s, err := parseStored(*st)
	if err != nil {
		e.d.Log("no server for the VPN lane: the selected server %s does not parse: %s", st.ID, connector.Mask(err.Error()))
		return Lane{}, reason(protocol.ReasonNoServer)
	}
	return Lane{Proxy: &s}, nil
}

// parseStored parses the saved text of a server again: one link, or a .conf named as when it was added.
func parseStored(st store.StoredServer) (connector.Server, error) {
	servers, rejected, address := connector.ParseText(st.Text, st.Name)
	switch {
	case len(rejected) > 0:
		return connector.Server{}, &rejected[0]
	case address != "" || len(servers) != 1:
		return connector.Server{}, errors.New("the text is not one server")
	}
	return servers[0], nil
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
// section 4). The connector becomes proxy with it only when Tailscale is not ready (not logged in, or no
// exit node selected) and routing is off or error with no operation in place, so that routing.enable uses
// the server; otherwise the server is selected for later and the connector stays: the user switches by
// choosing it (proxy.select). True when the connector became proxy: the setup mode is then stopped
// (stopSetupIfProxy). e.mu must be held.
func (e *Engine) selectFirstLocked(added []string) bool {
	if len(added) == 0 || e.serverLocked(e.saved.SelectedServer) != nil {
		return false
	}
	e.saved.SelectedServer = added[0]
	tailscaleReady := e.saved.LoggedIn && e.saved.SelectedExitNode != ""
	off := (e.st.Phase == protocol.PhaseOff || e.st.Phase == protocol.PhaseError) && e.op == nil
	switched := !tailscaleReady && off && connectorOf(e.saved) != protocol.ConnectorProxy
	if switched {
		e.saved.Connector = protocol.ConnectorProxy
		e.d.Log("the first server added: the connector is proxy (Tailscale is not ready)")
	}
	e.save()
	return switched
}

// stopSetupIfProxy stops the setup mode once the connector has become proxy in off or error (proxy.select,
// the first server added): the embedded node need not stay online in the tailnet for a connector that does
// not use it (stage 3 part 1, section 5); tailscale.login starts it again on demand. It waits for the setup
// core to end with the queue held and e.mu free, so that a tailscale.login after the command is not undone.
func (e *Engine) stopSetupIfProxy(switched bool) {
	if !switched {
		return
	}
	e.mu.Lock()
	running := e.setup != nil
	e.mu.Unlock()
	if running {
		e.d.Log("the connector is proxy: stopping the setup mode")
		e.stopSetup()
	}
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

// addResult is the answer of proxy.add and proxy.refresh: the servers new to the list, how many were there
// already, at most MaxRejected rejected lines and the number of the others.
func addResult(added []string, existing int, rejected []connector.Rejection) protocol.ProxyAddResult {
	r := protocol.ProxyAddResult{Added: append([]string{}, added...), Existing: existing, Rejected: []protocol.ProxyRejection{}}
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
// address (subscribe). The servers that are not there yet go into servers.json and are the answer's
// added; one that is there (the same id) stays as it is and counts as existing. An add that would take
// servers.json over its limit adds nothing: the rejection too_many of the whole text. The first server
// added becomes the selected one when none is (selectFirstLocked). Every phase but after helper.shutdown:
// nothing of it changes the running core.
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
	res, switched, err := e.addServersLocked(servers, rejected, source)
	e.mu.Unlock()
	e.stopSetupIfProxy(switched)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// addServersLocked is the part of proxy.add under e.mu, for links and a .conf; switched: the connector
// became proxy (selectFirstLocked).
func (e *Engine) addServersLocked(servers []connector.Server, rejected []connector.Rejection, source string) (protocol.ProxyAddResult, bool, *protocol.Error) {
	if err := e.proxyPhaseLocked(false); err != nil {
		return protocol.ProxyAddResult{}, false, err
	}
	if err := e.serversReadyLocked(); err != nil {
		return protocol.ProxyAddResult{}, false, err
	}
	next := cloneServers(e.servers)
	have := map[string]bool{}
	for _, s := range next.Servers {
		have[s.ID] = true
	}
	var added []string
	existing := 0
	for _, s := range servers {
		if have[s.ID] {
			existing++
			continue
		}
		have[s.ID] = true
		added = append(added, s.ID)
		next.Servers = append(next.Servers, storedServer(s, source))
	}
	if len(next.Servers) > store.MaxServers {
		e.d.Log("proxy.add: %d servers would be more than %d: nothing added", len(next.Servers), store.MaxServers)
		return addResult(nil, 0, append([]connector.Rejection{{Code: protocol.CodeTooMany}}, rejected...)), false, nil
	}
	if len(added) > 0 {
		if err := e.saveServersLocked(next); err != nil {
			return protocol.ProxyAddResult{}, false, err
		}
	}
	switched := e.selectFirstLocked(added)
	e.publish()
	e.d.Log("proxy.add: %d servers added, %d there already; %d lines rejected (%s)", len(added), existing, len(rejected), codes(rejected))
	return addResult(added, existing, rejected), switched, nil
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
// refusal costs no download; one download runs at a time (a second is busy). The download
// (connector.Fetcher: https only, no local address, 1 MB, 20 s) runs outside the queue of the
// state-changing commands, and the pipe answers other requests meanwhile; it ends with ctx (the client
// went, the helper closes the pipe), and then nothing changes. The queue is taken to apply what it gave,
// after the checks once more: a subscription that was there before the download and is gone after it was
// removed meanwhile, and is not made again (unknown_server). The servers of the subscription are replaced
// by those it gives now (one that another source holds stays with it); added are those new to the list,
// existing those that were there; the selected server stays selected when a server with its id is still
// there, else the selection goes. A download that fails, a subscription without a server and one that
// would take servers.json over its limit change no server: an existing subscription keeps the code in its
// error, which the state shows; proxy.add answers with the rejection of the whole text, proxy.refresh
// with the code as its error. The address is never in an answer, the state or the log.
func (e *Engine) subscribe(ctx context.Context, id, address string, add bool) (any, *protocol.Error) {
	e.mu.Lock()
	err := e.subscribePhaseLocked(id, add)
	existed := e.subscriptionLocked(id) != nil
	if err == nil && e.downloading {
		err = protocol.Errorf(protocol.CodeBusy, "another subscription is being downloaded")
	}
	if err == nil {
		e.downloading = true
	}
	e.mu.Unlock()
	if err != nil {
		return refusedSubscription(add, err)
	}
	body, ferr := e.d.Fetch.Get(ctx, address)
	e.mu.Lock()
	e.downloading = false
	e.mu.Unlock()
	if ctx.Err() != nil {
		e.d.Log("subscription %s: the download was given up, nobody waits for it", id)
		return nil, protocol.Errorf(protocol.CodeInternal, "the download was given up")
	}
	var servers []connector.Server
	var rejected []connector.Rejection
	if ferr == nil {
		servers, rejected = connector.ParseSubscription(body)
	}
	e.queue.Lock()
	defer e.queue.Unlock()
	e.mu.Lock()
	res, switched, err := e.applySubscriptionLocked(id, address, add, existed, servers, rejected, ferr)
	e.mu.Unlock()
	e.stopSetupIfProxy(switched)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// applySubscriptionLocked is the part of subscribe after the download, under the queue and e.mu; switched:
// the connector became proxy (selectFirstLocked).
func (e *Engine) applySubscriptionLocked(id, address string, add, existed bool, servers []connector.Server,
	rejected []connector.Rejection, ferr error) (any, bool, *protocol.Error) {
	// The phase or the servers changed during the download; a subscription there before is not made again.
	if err := e.subscribePhaseLocked(id, add && !existed); err != nil {
		res, rerr := refusedSubscription(add, err)
		return res, false, rerr
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
	count, existing := 0, 0
	if code == "" {
		kept := make([]store.StoredServer, 0, len(next.Servers)+len(servers))
		before, have := map[string]bool{}, map[string]bool{}
		for _, s := range next.Servers {
			before[s.ID] = true
			if s.Source != id {
				kept, have[s.ID] = append(kept, s), true
			}
		}
		for _, s := range servers {
			if before[s.ID] {
				existing++
			} else {
				added = append(added, s.ID)
			}
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
			return nil, false, protocol.Errorf(code, "subscription not refreshed: %s", code)
		}
		switch {
		case code == protocol.CodeTooMany:
			rejected = append([]connector.Rejection{{Code: code}}, rejected...)
		case ferr != nil || len(rejected) == 0:
			rejected = []connector.Rejection{{Code: code}}
		}
		return addResult(nil, 0, rejected), false, nil
	}
	sub := store.StoredSubscription{ID: id, Name: hostOf(address), URL: address,
		UpdatedAt: e.d.Clock.Now().UTC().Format(time.RFC3339), Count: count}
	if i := slices.IndexFunc(next.Subscriptions, func(s store.StoredSubscription) bool { return s.ID == id }); i >= 0 {
		next.Subscriptions[i] = sub
	} else {
		next.Subscriptions = append(next.Subscriptions, sub)
	}
	if err := e.saveServersLocked(next); err != nil {
		return nil, false, err
	}
	e.dropGoneSelectionLocked()
	switched := false
	if add {
		switched = e.selectFirstLocked(added)
	}
	e.publish()
	e.d.Log("subscription %s: %d servers, %d of them its own, %d new to the list; %d lines rejected (%s)",
		id, len(servers), count, len(added), len(rejected), codes(rejected))
	return addResult(added, existing, rejected), switched, nil
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
		return addResult(nil, 0, []connector.Rejection{{Code: protocol.CodeTooMany}}), nil
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
// (reconnectLocked); in off and error a change to proxy stops the setup mode (stopSetupIfProxy). The same
// server with the connector already proxy changes nothing. Stopping the setup mode waits for its core to end
// (stopProc: the graceful signal, then the forced end), so the answer, and the queue with it, may take up to
// about 2 × stopWait (10 s); a logout under way is finished first.
func (e *Engine) cmdProxySelect(params json.RawMessage) *protocol.Error {
	var p protocol.ProxyIDParams
	if err := protocol.DecodeParams(params, &p); err != nil {
		return err
	}
	if err := protocol.ValidateProxyID(p.ID); err != nil {
		return err
	}
	e.mu.Lock()
	switched, err := e.selectServerLocked(p.ID)
	e.mu.Unlock()
	e.stopSetupIfProxy(switched)
	return err
}

// selectServerLocked is proxy.select under e.mu; switched: the connector became proxy in off or error.
func (e *Engine) selectServerLocked(id string) (bool, *protocol.Error) {
	if err := e.reconnectPhaseLocked(); err != nil {
		return false, err
	}
	if err := e.serversReadyLocked(); err != nil {
		return false, err
	}
	if e.serverLocked(id) == nil {
		return false, protocol.Errorf(protocol.CodeUnknownServer, "no server has this id")
	}
	wasProxy := connectorOf(e.saved) == protocol.ConnectorProxy
	if e.saved.SelectedServer == id && wasProxy {
		return false, nil
	}
	e.saved.SelectedServer, e.saved.Connector = id, protocol.ConnectorProxy
	e.save()
	if !live(e.st.Phase) {
		e.publish()
		return !wasProxy, nil
	}
	e.reconnectLocked()
	return false, nil
}
