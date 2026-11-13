package singbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"splitlane/helper/internal/engine"
)

// Clash is the client of the core's Clash API (Plan 0, "Факты о ядре 1.14.4").
type Clash struct {
	base   string
	secret string
	http   *http.Client
}

var _ engine.ClashAPI = (*Clash)(nil)

// NewClash talks to base ("http://127.0.0.1:<port>") with the run's secret, never through a proxy.
func NewClash(base, secret string) *Clash {
	return &Clash{base: base, secret: secret, http: &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{Proxy: nil},
	}}
}

func (c *Clash) do(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("clash api %s %s: HTTP %d", method, path, resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// SetMode is PATCH /configs, then GET /configs: the core answers 204 even for a mode it does not
// know, so the switch counts only when the mode reads back (spec 6.3).
func (c *Clash) SetMode(ctx context.Context, mode string) error {
	if err := c.do(ctx, http.MethodPatch, "/configs", map[string]string{"mode": mode}, nil); err != nil {
		return err
	}
	var cfg struct {
		Mode string `json:"mode"`
	}
	if err := c.do(ctx, http.MethodGet, "/configs", nil, &cfg); err != nil {
		return err
	}
	if !strings.EqualFold(cfg.Mode, mode) {
		return fmt.Errorf("the core did not take mode %s (it is in %s)", mode, cfg.Mode)
	}
	return nil
}

// Connections is GET /connections: id, process path, the lane (the first of "chains" is the
// outbound, and the config names the outbounds after the lanes) and whether rules 8–11 chose it.
func (c *Clash) Connections(ctx context.Context) ([]engine.Conn, error) {
	var data struct {
		Connections []struct {
			ID       string `json:"id"`
			Metadata struct {
				ProcessPath string `json:"processPath"`
			} `json:"metadata"`
			Chains []string `json:"chains"`
			Rule   string   `json:"rule"`
		} `json:"connections"`
	}
	if err := c.do(ctx, http.MethodGet, "/connections", nil, &data); err != nil {
		return nil, err
	}
	out := make([]engine.Conn, 0, len(data.Connections))
	for _, x := range data.Connections {
		conn := engine.Conn{ID: x.ID, ProcessPath: x.Metadata.ProcessPath, ByLanes: byLanes(x.Rule)}
		if len(x.Chains) > 0 {
			conn.Lane = x.Chains[0]
		}
		out = append(out, conn)
	}
	return out, nil
}

// byLanes reads the "rule" field: the text of the rule and its action. Rules 8–11 of 6.2 are a
// mode rule, a lane file or the final route: "clash_mode=vpn => route(vpn)",
// "rule_set=lane-direct => route(direct)", "final" (Plan 1A, "Факты, на которые опирается план").
func byLanes(rule string) bool {
	return strings.HasPrefix(rule, "clash_mode=") || strings.HasPrefix(rule, "rule_set=lane-") || rule == "final"
}

// CloseConn is DELETE /connections/<id>. DELETE /connections without an id is never used:
// it resets the whole network of the core (spec 6.3).
func (c *Clash) CloseConn(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("empty connection id")
	}
	return c.do(ctx, http.MethodDelete, "/connections/"+url.PathEscape(id), nil, nil)
}
