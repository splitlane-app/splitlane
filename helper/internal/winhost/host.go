// Package winhost answers the engine's questions about Windows: routes, IP forwarding, the NRPT
// rule of the system Tailscale, a running sing-box, the adapter, the DNS cache (spec 5.3, 5.4).
// The decisions are pure functions over plain data; the readers only read the system.
package winhost

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/engine"
)

// Iface is one network interface as the decisions see it.
type Iface struct {
	LUID       uint64
	Alias      string
	Type       uint32
	Up         bool
	Hardware   bool // a physical adapter
	Forwarding bool // IPv4 forwarding enabled
}

// Route is one IPv4 route.
type Route struct {
	LUID      uint64
	PrefixLen uint8
}

// NRPTRule is one rule of the Name Resolution Policy Table.
type NRPTRule struct {
	Names   []string
	Servers string
}

// ignored adapters: the core's own and the system Tailscale's.
func ignored(i Iface) bool {
	return strings.EqualFold(i.Alias, coreconfig.AdapterName) || strings.EqualFold(i.Alias, "Tailscale") ||
		i.Type == windows.IF_TYPE_SOFTWARE_LOOPBACK
}

// DefaultRouteOK: an IPv4 default route through an adapter that is up and neither ours nor Tailscale's;
// an adapter with its cable out does not count, whatever the route table still holds.
func DefaultRouteOK(routes []Route, ifaces []Iface) bool {
	byLUID := map[uint64]Iface{}
	for _, i := range ifaces {
		byLUID[i.LUID] = i
	}
	for _, r := range routes {
		i, ok := byLUID[r.LUID]
		if r.PrefixLen == 0 && ok && i.Up && !ignored(i) {
			return true
		}
	}
	return false
}

// PhysicalForwarding: a physical adapter that is up forwards IPv4 (Internet sharing, mobile hotspot).
func PhysicalForwarding(ifaces []Iface) bool {
	for _, i := range ifaces {
		if i.Hardware && i.Up && i.Forwarding && !ignored(i) {
			return true
		}
	}
	return false
}

// TailscaleCatchAll: the system Tailscale's NRPT rule for all names ("."), served by 100.100.100.100.
func TailscaleCatchAll(rules []NRPTRule) bool {
	for _, r := range rules {
		for _, n := range r.Names {
			if n == "." && strings.Contains(r.Servers, "100.100.100.100") {
				return true
			}
		}
	}
	return false
}

// Host implements engine.Host on the real system.
type Host struct{}

var _ engine.Host = Host{}

func (Host) HasDefaultRoute() bool {
	ifaces, err := Interfaces()
	if err != nil {
		return false
	}
	routes, err := DefaultRoutes()
	if err != nil {
		return false
	}
	return DefaultRouteOK(routes, ifaces)
}

func (Host) IPForwarding() bool {
	ifaces, err := Interfaces()
	return err == nil && PhysicalForwarding(ifaces)
}

func (Host) DNSOverride() bool { return TailscaleCatchAll(ReadNRPT()) }

func (Host) CoreRunning() bool {
	n, _ := CountProcesses("sing-box.exe")
	return n > 0
}

func (Host) AdapterPresent() bool {
	ifaces, err := Interfaces()
	if err != nil {
		return true // unknown counts as present: the rollback must not report a clean system by mistake
	}
	for _, i := range ifaces {
		if strings.EqualFold(i.Alias, coreconfig.AdapterName) {
			return true
		}
	}
	return false
}

var (
	dnsapi                    = windows.NewLazySystemDLL("dnsapi.dll")
	procDnsFlushResolverCache = dnsapi.NewProc("DnsFlushResolverCache")
)

// FlushDNS empties the Windows DNS cache (spec 5.3 step 7, 6.4).
func (Host) FlushDNS() error {
	if r, _, err := procDnsFlushResolverCache.Call(); r == 0 {
		return err
	}
	return nil
}

// FreePorts takes n free ports on 127.0.0.1 at once and lets them go.
func (Host) FreePorts(n int) ([]int, error) {
	var ports []int
	var ls []net.Listener
	defer func() {
		for _, l := range ls {
			l.Close()
		}
	}()
	for i := 0; i < n; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		ls = append(ls, l)
		ports = append(ports, l.Addr().(*net.TCPAddr).Port)
	}
	return ports, nil
}

// Secret is a fresh random secret for the Clash API or the API service (spec 6.1).
func (Host) Secret() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Interfaces reads every interface with its IPv4 forwarding flag.
func Interfaces() ([]Iface, error) {
	size := uint32(16 * 1024)
	var buf []byte
	for {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, windows.GAA_FLAG_SKIP_ANYCAST|windows.GAA_FLAG_SKIP_MULTICAST|windows.GAA_FLAG_SKIP_DNS_SERVER, 0,
			(*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) || size <= uint32(len(buf)) {
			return nil, err
		}
	}
	var out []Iface
	for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); a != nil; a = a.Next {
		i := Iface{LUID: a.Luid, Alias: windows.UTF16PtrToString(a.FriendlyName), Type: a.IfType, Up: a.OperStatus == windows.IfOperStatusUp}
		row := windows.MibIfRow2{InterfaceLuid: a.Luid}
		if windows.GetIfEntry2Ex(windows.MibIfTableNormalWithoutStatistics, &row) == nil {
			i.Hardware = row.InterfaceAndOperStatusFlags&1 != 0
		}
		ip := windows.MibIpInterfaceRow{Family: windows.AF_INET, InterfaceLuid: a.Luid}
		if windows.GetIpInterfaceEntry(&ip) == nil {
			i.Forwarding = ip.ForwardingEnabled != 0
		}
		out = append(out, i)
	}
	return out, nil
}

// DefaultRoutes reads the IPv4 routes with prefix length 0.
func DefaultRoutes() ([]Route, error) {
	var table *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_INET, &table); err != nil {
		return nil, err
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))
	var out []Route
	for _, r := range table.Rows() {
		if r.DestinationPrefix.PrefixLength == 0 {
			out = append(out, Route{LUID: r.InterfaceLuid, PrefixLen: 0})
		}
	}
	return out, nil
}

// ReadNRPT reads the NRPT rules of both places Windows keeps them.
func ReadNRPT() []NRPTRule {
	var out []NRPTRule
	for _, path := range []string{
		`SYSTEM\CurrentControlSet\Services\Dnscache\Parameters\DnsPolicyConfig`,
		`SOFTWARE\Policies\Microsoft\Windows NT\DNSClient\DnsPolicyConfig`,
	} {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.READ)
		if err != nil {
			continue
		}
		names, _ := k.ReadSubKeyNames(-1)
		for _, n := range names {
			sk, err := registry.OpenKey(k, n, registry.READ)
			if err != nil {
				continue
			}
			var r NRPTRule
			r.Names, _, _ = sk.GetStringsValue("Name")
			r.Servers, _, _ = sk.GetStringValue("GenericDNSServers")
			sk.Close()
			out = append(out, r)
		}
		k.Close()
	}
	return out
}

// CountProcesses counts running processes with this image name (case-insensitive).
func CountProcesses(image string) (int, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	n := 0
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), image) {
			n++
		}
	}
	return n, nil
}
