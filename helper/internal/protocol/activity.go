package protocol

import "encoding/json"

// EventActivity is the summary of the live activity, sent once a second (spec 5.1, 8.4).
const EventActivity = "activity"

// Kinds of an activity group (spec 8.4). The kind of the "Windows services" group is KindSystem,
// the same word as the rule kind of that tile.
const (
	KindApp          = "app"
	KindUnattributed = "unattributed"
)

// Keys of the special tiles of unpinned traffic (spec 8.4). Every other key is a rule id or a program folder.
const (
	KeySystem       = "system"
	KeyUnattributed = "unattributed"
)

// Traffic is a rate: bytes per second over the last second.
type Traffic struct {
	Down int64 `json:"down"`
	Up   int64 `json:"up"`
}

// ActivityLanes are the rates of the two lanes.
type ActivityLanes struct {
	VPN    Traffic `json:"vpn"`
	Direct Traffic `json:"direct"`
}

// ActivityApp is one group of the summary: a pinned program, an unpinned program folder or a special tile.
type ActivityApp struct {
	Key         string  `json:"key"`
	Kind        string  `json:"kind"` // KindApp, KindSystem, KindUnattributed
	Name        string  `json:"name"`
	Exe         string  `json:"exe"`
	RuleID      *string `json:"ruleId"`
	Lane        string  `json:"lane"`
	Down        int64   `json:"down"`
	Up          int64   `json:"up"`
	Connections int     `json:"connections"`
}

// ActivityConn is one connection of the group named by detailFor.
type ActivityConn struct {
	Host string `json:"host"`
	Lane string `json:"lane"`
	Down int64  `json:"down"`
	Up   int64  `json:"up"`
}

// ActivityDetail is the detail of one group: the executables seen in it and its connections now.
type ActivityDetail struct {
	Key         string         `json:"key"`
	Exes        []string       `json:"exes"`
	Connections []ActivityConn `json:"connections"`
}

// Activity is the data of the activity event (spec 8.4). Detail is there only for a subscription with detailFor.
type Activity struct {
	Lanes  ActivityLanes   `json:"lanes"`
	Apps   []ActivityApp   `json:"apps"`
	Detail *ActivityDetail `json:"detail,omitempty"`
}

// MarshalJSON writes a missing list of apps as [], never null.
func (a Activity) MarshalJSON() ([]byte, error) {
	type plain Activity
	p := plain(a)
	if p.Apps == nil {
		p.Apps = []ActivityApp{}
	}
	return json.Marshal(p)
}

// MarshalJSON writes missing lists as [], never null.
func (d ActivityDetail) MarshalJSON() ([]byte, error) {
	type plain ActivityDetail
	p := plain(d)
	if p.Exes == nil {
		p.Exes = []string{}
	}
	if p.Connections == nil {
		p.Connections = []ActivityConn{}
	}
	return json.Marshal(p)
}

// SubscribeParams are the params of activity.subscribe.
type SubscribeParams struct {
	DetailFor string `json:"detailFor"`
}

// DecodeSubscribe reads the params of activity.subscribe: {} or {"detailFor": "<key>"} (spec 5.1, 8.4).
// Unknown fields and a key longer than MaxPathLen are bad_request; "" asks for no detail.
func DecodeSubscribe(params json.RawMessage) (detailFor string, err *Error) {
	var p SubscribeParams
	if e := DecodeParams(params, &p); e != nil {
		return "", e
	}
	if len(p.DetailFor) > MaxPathLen {
		return "", Errorf(CodeBadRequest, "detailFor is longer than %d bytes", MaxPathLen)
	}
	return p.DetailFor, nil
}
