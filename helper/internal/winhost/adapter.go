package winhost

import (
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/coreconfig"
)

// netClass is GUID_DEVCLASS_NET, the device class of network adapters.
var netClass = windows.GUID{Data1: 0x4d36e972, Data2: 0xe325, Data3: 0x11ce, Data4: [8]byte{0xbf, 0xc1, 0x08, 0x00, 0x2b, 0xe1, 0x03, 0x18}}

// wintunEnumerator is the enumerator of the devices Wintun creates (software devices).
const wintunEnumerator = `SWD\Wintun`

// AdapterGUID is the GUID sing-tun asks Wintun for when it creates the adapter of this name: the
// MD5 of "wintun" and the name, its 16 bytes read as a GUID (sing-tun, generateGUIDByDeviceName).
func AdapterGUID(name string) windows.GUID {
	sum := md5.Sum([]byte("wintun" + name))
	return windows.GUID{
		Data1: binary.LittleEndian.Uint32(sum[0:4]),
		Data2: binary.LittleEndian.Uint16(sum[4:6]),
		Data3: binary.LittleEndian.Uint16(sum[6:8]),
		Data4: [8]byte(sum[8:16]),
	}
}

// AdapterInstanceID is the device instance id of that adapter: SWD\WINTUN\{GUID}.
func AdapterInstanceID(name string) string {
	return `SWD\WINTUN\` + AdapterGUID(name).String()
}

// WintunDevice is one network device that Wintun created. Present is false for a device Windows
// keeps although nothing runs it: a forced end of the core leaves its device so.
type WintunDevice struct {
	InstanceID string
	Present    bool
}

// devices is the device list of Windows as removeStale uses it; the tests use a fake.
type devices interface {
	Wintun() ([]WintunDevice, error)
	Remove(instanceID string) error
}

// removeStale removes the device of the adapter name when it is there and not present, and
// nothing else: a present device belongs to a running core, any other device to another program.
func removeStale(d devices, name string) (bool, error) {
	list, err := d.Wintun()
	if err != nil {
		return false, err
	}
	id := AdapterInstanceID(name)
	for _, dev := range list {
		if !strings.EqualFold(dev.InstanceID, id) {
			continue
		}
		if dev.Present {
			return false, nil
		}
		if err := d.Remove(dev.InstanceID); err != nil {
			return false, fmt.Errorf("remove %s: %w", dev.InstanceID, err)
		}
		return true, nil
	}
	return false, nil
}

// RemoveStaleAdapter removes the Wintun device of the core's adapter that a forced end of an
// earlier core left behind, non-present: with it, the next core may fail to create its adapter
// (spec 5.3 step 4; appendix A, checks of plan 1B). The removal needs administrator rights.
func (Host) RemoveStaleAdapter() (bool, error) {
	return removeStale(setupAPI{}, coreconfig.AdapterName)
}

// setupAPI is the device list of Windows (SetupAPI).
type setupAPI struct{}

// Wintun lists the Wintun devices, present or not; present are those a listing with
// DIGCF_PRESENT also has.
func (setupAPI) Wintun() ([]WintunDevice, error) {
	all, err := wintunIDs(0)
	if err != nil {
		return nil, err
	}
	present, err := wintunIDs(windows.DIGCF_PRESENT)
	if err != nil {
		return nil, err
	}
	out := make([]WintunDevice, 0, len(all))
	for _, id := range all {
		d := WintunDevice{InstanceID: id}
		for _, p := range present {
			d.Present = d.Present || strings.EqualFold(p, id)
		}
		out = append(out, d)
	}
	return out, nil
}

// Remove removes the Wintun device with this instance id, present or not, as Device Manager
// uninstalls a device: DIF_REMOVE in all hardware profiles.
func (setupAPI) Remove(instanceID string) error {
	set, err := windows.SetupDiGetClassDevsEx(&netClass, wintunEnumerator, 0, 0, 0, "")
	if err != nil {
		return err
	}
	defer set.Close()
	for i := 0; ; i++ {
		data, err := set.EnumDeviceInfo(i)
		if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
			return fmt.Errorf("no device %s", instanceID)
		}
		if err != nil {
			return err
		}
		if id, err := set.DeviceInstanceID(data); err != nil || !strings.EqualFold(id, instanceID) {
			continue
		}
		params := windows.RemoveDeviceParams{
			ClassInstallHeader: *windows.MakeClassInstallHeader(windows.DIF_REMOVE),
			Scope:              windows.DI_REMOVEDEVICE_GLOBAL,
		}
		if err := set.SetClassInstallParams(data, &params.ClassInstallHeader, uint32(unsafe.Sizeof(params))); err != nil {
			return err
		}
		return set.CallClassInstaller(windows.DIF_REMOVE, data)
	}
}

// wintunIDs lists the instance ids of the Wintun devices of the Net class; with DIGCF_PRESENT
// only the present ones.
func wintunIDs(flags windows.DIGCF) ([]string, error) {
	set, err := windows.SetupDiGetClassDevsEx(&netClass, wintunEnumerator, 0, flags, 0, "")
	if err != nil {
		return nil, err
	}
	defer set.Close()
	var ids []string
	for i := 0; ; i++ {
		data, err := set.EnumDeviceInfo(i)
		if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
			return ids, nil
		}
		if err != nil {
			return nil, err
		}
		id, err := set.DeviceInstanceID(data)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
}
