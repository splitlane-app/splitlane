package diag

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"testing"

	"splitlane/helper/internal/connector"
)

// coreLogText is a made-up core log of about n bytes: the lines sing-box writes while it runs, now and then
// one with a link, a key or a UUID in it.
func coreLogText(n int) string {
	lines := []string{
		"+0000 2026-11-16 12:00:%02d INFO [%d 0ms] inbound/tun[tun-in]: inbound connection from 172.19.0.1:%d",
		"+0000 2026-11-16 12:00:%02d INFO [%d 12ms] outbound/vless[vpn]: outbound connection to www.example.com:443 (%d)",
		"+0000 2026-11-16 12:00:%02d DEBUG [%d 3ms] dns: exchanged A api.ipify.org. 3600 IN A 192.0.2.%d",
		"+0000 2026-11-16 12:00:%02d ERROR [%d 20s] connection: open connection to 198.51.100.7:443 using outbound/vless[vpn]: dial tcp: i/o timeout (%d)",
		"+0000 2026-11-16 12:00:%02d INFO [%d 1ms] router: match[%d] rule_set=lane-vpn => route(vpn)",
		"+0000 2026-11-16 12:00:%02d ERROR [%d 0ms] outbound/vless[vpn]: user 3f1b2c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d rejected (%d)",
		"+0000 2026-11-16 12:00:%02d ERROR [%d 0ms] parse vless://3f1b2c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d@192.0.2.10:443?pbk=ezvd-drM4QNV61tfmh5v-OP6yhTnAAQyxi5VK7zbfUo#x (%d)",
		"+0000 2026-11-16 12:00:%02d WARN [%d 0ms] endpoint/wireguard[vpn]: peer lhw1Rezx8+44v1b7GrtW7HxgVikNwpLsRNzZk0ZbBXA= handshake (%d)",
		`+0000 2026-11-16 12:00:%02d ERROR [%d 0ms] decode: {"password":"hunter2secretpass","n":%d}`,
	}
	var b strings.Builder
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, lines[i%len(lines)]+"\n", i%60, i, i%250)
	}
	return b.String()
}

// maskCoreLog is connector.Mask line by line on the lines that may hold a secret: every line comes out as
// Mask makes it on its own, and a line that the prefilter passes by is one Mask does not change.
func TestMaskCoreLogIsMaskByLine(t *testing.T) {
	text := coreLogText(64 << 10)
	var want strings.Builder
	for _, l := range strings.SplitAfter(text, "\n") {
		want.WriteString(connector.Mask(l))
	}
	if got := string(maskCoreLog([]byte(text))); got != want.String() {
		t.Fatal("maskCoreLog differs from Mask line by line")
	}
	for _, secret := range []string{"3f1b2c4d-5e6f", "ezvd-drM4QNV61", "lhw1Rezx8", "hunter2secretpass"} {
		if strings.Contains(string(maskCoreLog([]byte(text))), secret) {
			t.Errorf("%s is left", secret)
		}
	}
	// Lines of the characters Mask reacts to, but without "://", '=' and '"': where the prefilter passes a
	// line by, Mask leaves it as it is.
	r := rand.New(rand.NewSource(1))
	const alphabet = "0123456789abcdefABCDEF-/@.?&;#_+ :xyzQ\t"
	for range 20000 {
		var b strings.Builder
		for range 10 + r.Intn(120) {
			b.WriteByte(alphabet[r.Intn(len(alphabet))])
		}
		if r.Intn(4) == 0 {
			b.WriteString("1f2e3d4c-5b6a-7c8d-9e0f-a1b2c3d4e5f6"[r.Intn(10):])
		}
		l := b.String()
		if !mayHoldSecret([]byte(l)) && connector.Mask(l) != l {
			t.Fatalf("Mask changes a line the prefilter passes by: %q -> %q", l, connector.Mask(l))
		}
	}
}

// BenchmarkMaskCoreLog: the masking of an 8 MiB core log, with the prefilter and, for comparison, Mask over
// all of it (stage 3 part 1, review of task 5: an export takes up to 4 such logs, 32 MiB).
func BenchmarkMaskCoreLog(b *testing.B) {
	text := []byte(coreLogText(8 << 20))
	b.Run("prefilter", func(b *testing.B) {
		b.SetBytes(int64(len(text)))
		for range b.N {
			maskCoreLog(text)
		}
	})
	b.Run("whole", func(b *testing.B) {
		b.SetBytes(int64(len(text)))
		for range b.N {
			connector.Mask(string(text))
		}
	})
}

// BenchmarkBuild32MiB: an archive with four core logs of 8 MiB each.
func BenchmarkBuild32MiB(b *testing.B) {
	dir := b.TempDir()
	text := coreLogText(8 << 20)
	var logs []string
	for _, name := range []string{"core.log", "core.log.prev", "core-setup.log", "core-setup.log.prev"} {
		logs = append(logs, write(b, filepath.Join(dir, "logs", name), text))
	}
	for i := range b.N {
		if err := Build(filepath.Join(dir, "export", fmt.Sprintf("%d.zip", i)), Input{Logs: logs, State: []byte("{}\n")}); err != nil {
			b.Fatal(err)
		}
	}
}

// plainCoreLogText is a core log of about n bytes as the core writes it at level info: connection lines,
// one line in a hundred with a link or a key in an error.
func plainCoreLogText(n int) string {
	dense := strings.Split(coreLogText(4<<10), "\n")
	var b strings.Builder
	for i := 0; b.Len() < n; i++ {
		switch {
		case i%100 == 99:
			b.WriteString(dense[5+i%4] + "\n")
		case i%2 == 0:
			fmt.Fprintf(&b, "+0000 2026-11-16 12:00:%02d INFO [%d 0ms] inbound/tun[tun-in]: inbound connection from 172.19.0.1:%d\n", i%60, i, 40000+i%20000)
		default:
			fmt.Fprintf(&b, "+0000 2026-11-16 12:00:%02d INFO [%d 12ms] outbound/vless[vpn]: outbound connection to www.example.com:443\n", i%60, i)
		}
	}
	return b.String()
}

// BenchmarkMaskPlainCoreLog: maskCoreLog over an 8 MiB core log of level info.
func BenchmarkMaskPlainCoreLog(b *testing.B) {
	text := []byte(plainCoreLogText(8 << 20))
	b.SetBytes(int64(len(text)))
	for range b.N {
		maskCoreLog(text)
	}
}
