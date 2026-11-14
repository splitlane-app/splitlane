package winhost

import (
	"os"
	"strings"
	"testing"

	"splitlane/helper/internal/coreconfig"
)

// Both vectors were read on the PC: the instance ids of the devices sing-tun created for these names.
func TestAdapterGUIDVectors(t *testing.T) {
	for name, want := range map[string]string{
		"SplitLane":   "{5C76B3EA-99F5-8F54-B2FD-465779C01F24}",
		"RouterSpike": "{5A9409FA-A8AE-9CFA-F352-D298E1BD5312}",
	} {
		if got := AdapterGUID(name).String(); got != want {
			t.Errorf("%s: GUID %s, want %s", name, got, want)
		}
		if got := AdapterInstanceID(name); got != `SWD\WINTUN\`+want {
			t.Errorf("%s: instance id %s", name, got)
		}
	}
}

// fakeDevices is the device list of the decision; removed records every removal it was asked for.
type fakeDevices struct {
	list      []WintunDevice
	listErr   error
	removeErr error
	removed   []string
}

func (f *fakeDevices) Wintun() ([]WintunDevice, error) { return f.list, f.listErr }

func (f *fakeDevices) Remove(instanceID string) error {
	f.removed = append(f.removed, instanceID)
	return f.removeErr
}

// Only a non-present device with the instance id of our adapter goes; nothing else is touched.
func TestStaleDecision(t *testing.T) {
	ours, other := AdapterInstanceID("SplitLane"), AdapterInstanceID("RouterSpike")
	for name, c := range map[string]struct {
		list []WintunDevice
		gone string // the device that must be removed, "" for none
	}{
		"present: a core runs":        {list: []WintunDevice{{InstanceID: ours, Present: true}}},
		"absent":                      {list: []WintunDevice{{InstanceID: other, Present: true}}},
		"non-present, ours":           {list: []WintunDevice{{InstanceID: other, Present: true}, {InstanceID: ours}}, gone: ours},
		"non-present, another's":      {list: []WintunDevice{{InstanceID: other}}},
		"no Wintun device at all":     {},
		"non-present, ours, any case": {list: []WintunDevice{{InstanceID: strings.ToLower(ours)}}, gone: strings.ToLower(ours)},
	} {
		d := &fakeDevices{list: c.list}
		removed, err := removeStale(d, "SplitLane")
		if err != nil || removed != (c.gone != "") || strings.Join(d.removed, ",") != c.gone {
			t.Errorf("%s: removed %v, err %v, removals %v", name, removed, err, d.removed)
		}
	}
	d := &fakeDevices{list: []WintunDevice{{InstanceID: ours}}, removeErr: os.ErrPermission}
	if removed, err := removeStale(d, "SplitLane"); removed || err == nil || !strings.Contains(err.Error(), ours) {
		t.Fatalf("a failed removal: removed %v, err %v", removed, err)
	}
	d = &fakeDevices{listErr: os.ErrPermission}
	if removed, err := removeStale(d, "SplitLane"); removed || err == nil || len(d.removed) != 0 {
		t.Fatalf("a failed listing: removed %v, err %v, removals %v", removed, err, d.removed)
	}
}

// TestLiveWintunListReadOnly lists the Wintun devices of the real system and removes nothing: the
// decision runs on the real list with the fake removal. It runs with SPLITLANE_LIVE_READONLY=1.
func TestLiveWintunListReadOnly(t *testing.T) {
	if os.Getenv("SPLITLANE_LIVE_READONLY") != "1" {
		t.Skip("SPLITLANE_LIVE_READONLY is not set")
	}
	list, err := setupAPI{}.Wintun()
	if err != nil {
		t.Fatalf("wintun devices: %v", err)
	}
	for _, d := range list {
		t.Logf("LIVE wintun %s present=%v", d.InstanceID, d.Present)
	}
	removed, err := removeStale(&fakeDevices{list: list}, coreconfig.AdapterName)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("LIVE wintun %d devices, stale device of %s: %v", len(list), coreconfig.AdapterName, removed)
}
