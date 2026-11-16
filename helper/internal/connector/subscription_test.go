package connector

import (
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
)

var (
	linkVLESS  = "vless://" + uuid1 + "@192.0.2.10:443?security=reality&pbk=" + pbk + "&sni=www.example.com&fp=chrome&flow=xtls-rprx-vision#%F0%9F%87%A9%F0%9F%87%AA%20Germany"
	linkTrojan = "trojan://Pa55-w0rd@example.com:443?sni=example.com#Trojan"
	linkSS     = "ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpFeDRtcGxlLVBhc3M@192.0.2.5:8388/?outline=1#Outline"
	linkHy2    = "hy2://letmein@example.com:443?sni=example.com#hy2"
	linkPlugin = "ss://YWVzLTI1Ni1nY206cGFzcw@example.com:8388/?plugin=obfs-local%3Bobfs%3Dhttp"
)

type rej struct {
	line int
	code string
}

func rejs(rs []Rejection) []rej {
	var out []rej
	for _, r := range rs {
		out = append(out, rej{r.Line, r.Code})
	}
	return out
}

func names(ss []Server) []string {
	var out []string
	for _, s := range ss {
		out = append(out, s.Name)
	}
	return out
}

func TestParseText(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		servers []string
		rejects []rej
		sub     string
	}{
		{"links, blank lines, comments, a bad line and a plugin",
			"\n# my servers\r\n" + linkVLESS + "\r\n\r\n  " + linkTrojan + "  \nnot a link\n" + linkPlugin + "\n" + linkSS + "\n",
			[]string{"🇩🇪 Germany", "Trojan", "Outline"}, []rej{{6, CodeBadLink}, {7, CodePlugin}}, ""},
		{"one link", linkHy2, []string{"hy2"}, nil, ""},
		{"duplicate links count once", linkTrojan + "\n" + linkTrojan + "\n  " + linkTrojan, []string{"Trojan"}, nil, ""},
		{"the same server under two names counts once", linkTrojan + "\n" + strings.Replace(linkTrojan, "#Trojan", "#Trojan 2", 1), []string{"Trojan"}, nil, ""},
		{"a line that is not utf-8", linkTrojan + "\n\xff\xfe\n" + linkSS, []string{"Trojan", "Outline"}, []rej{{2, CodeBadLink}}, ""},
		{"subscription address alone", "\n  https://sub.example.com/api/v1/client/subscribe?token=abc123  \n", nil, nil,
			"https://sub.example.com/api/v1/client/subscribe?token=abc123"},
		{"subscription address with a comment", "# my provider\nhttps://sub.example.com/s/x\n", nil, nil, "https://sub.example.com/s/x"},
		{"subscription address among links", linkTrojan + "\nhttps://sub.example.com/s/x\n", []string{"Trojan"}, []rej{{2, CodeBadLink}}, ""},
		{"two subscription addresses", "https://a.example.com/s\nhttps://b.example.com/s", nil, []rej{{1, CodeBadLink}, {2, CodeBadLink}}, ""},
		{"bad subscription address", "https://", nil, []rej{{1, CodeBadLink}}, ""},
		{"subscription address with a space", "https://sub.example.com/a b", nil, []rej{{1, CodeBadLink}}, ""},
		{"plain http subscription", "http://sub.example.com/s", nil, []rej{{0, CodeFetchNotHTTPS}}, ""},
		{"plain http subscription with a comment, upper case", "# my provider\n  HTTP://sub.example.com/s?token=abc123 \n", nil,
			[]rej{{0, CodeFetchNotHTTPS}}, ""},
		{"http address among links", linkTrojan + "\nhttp://sub.example.com/s\n", []string{"Trojan"}, []rej{{2, CodeUnsupportedProtocol}}, ""},
		{"wireguard text", "\r\n" + strings.ReplaceAll(wgConf, "\n", "\r\n"), []string{"vpn.example.com:51820"}, nil, ""},
		{"wireguard text with an error", strings.Replace(wgConf, "MTU = 1420", "Jc = 3", 1), nil, []rej{{5, CodeAmneziaWG}}, ""},
		{"only comments", "# nothing\n\n#\n", nil, nil, ""},
		{"empty", "", nil, nil, ""},
		{"too long", linkTrojan + "\n" + strings.Repeat("#", MaxTextLen), nil, []rej{{0, CodeTooLong}}, ""},
		{"too long line", "#ok\n" + linkTrojan + "&x=" + strings.Repeat("a", MaxLinkLen), nil, []rej{{2, CodeTooLong}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			servers, rejected, sub := ParseText(c.text, "")
			if got := names(servers); !reflect.DeepEqual(got, c.servers) {
				t.Errorf("servers %q, want %q", got, c.servers)
			}
			if got := rejs(rejected); !reflect.DeepEqual(got, c.rejects) {
				t.Errorf("rejected %v, want %v", got, c.rejects)
			}
			if sub != c.sub {
				t.Errorf("subscription %q, want %q", sub, c.sub)
			}
			for _, r := range rejected {
				if strings.Contains(r.Detail, "abc123") || strings.Contains(r.Detail, "sub.example.com") {
					t.Errorf("a rejection repeats the address: %+v", r)
				}
			}
		})
	}
}

// A .conf pasted together with links (the kit prints both): each part is read as if pasted alone, with
// the line numbers of the whole text, and the .conf gets the id it has alone.
func TestParseTextConfWithLinks(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		servers []string
		rejects []rej
	}{
		{"links, then the conf", linkVLESS + "\n" + linkSS + "\n\n" + wgConf, []string{"vpn.example.com:51820", "🇩🇪 Germany", "Outline"}, nil},
		{"the conf, then links", wgConf + "\n" + linkTrojan, []string{"vpn.example.com:51820", "Trojan"}, nil},
		{"a bad link keeps its line", linkTrojan + "\nvless://broken\n" + wgConf, []string{"vpn.example.com:51820", "Trojan"}, []rej{{2, CodeMissingField}}},
		{"a bad conf keeps its line, the links still count", linkTrojan + "\n" + strings.Replace(wgConf, "MTU = 1420", "Jc = 3", 1),
			[]string{"Trojan"}, []rej{{6, CodeAmneziaWG}}},
		{"a comment with an address stays in the conf", "# from https://example.com/vpn\n" + wgConf, []string{"vpn.example.com:51820"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			servers, rejected, sub := ParseText(c.text, "")
			if got := names(servers); !reflect.DeepEqual(got, c.servers) {
				t.Errorf("servers %q, want %q", got, c.servers)
			}
			if got := rejs(rejected); !reflect.DeepEqual(got, c.rejects) {
				t.Errorf("rejected %v, want %v", got, c.rejects)
			}
			if sub != "" {
				t.Errorf("subscription %q", sub)
			}
		})
	}
	alone, _, _ := ParseText(wgConf, "")
	mixed, _, _ := ParseText(linkTrojan+"\n"+wgConf+"\n"+linkSS, "")
	key := []byte("0123456789abcdef0123456789abcdef")
	if len(alone) != 1 || len(mixed) != 3 || alone[0].Text != mixed[0].Text || ServerID(key, alone[0].Text) != ServerID(key, mixed[0].Text) {
		t.Fatalf("the conf among links differs from the conf alone: %s / %s", dump(alone), dump(mixed))
	}
}

func TestParseTextConfName(t *testing.T) {
	cases := []struct{ conf, want string }{
		{"office", "office"},
		{"home.conf", "home"},
		{"Дом.CONF", "Дом"},
		{"configs/sub/home.conf", "home"},
		{`configs\home.conf`, "home"},
		{"  ", "vpn.example.com:51820"},
		{"", "vpn.example.com:51820"},
	}
	for _, c := range cases {
		servers, rejected, _ := ParseText(wgConf, c.conf)
		if len(servers) != 1 || len(rejected) != 0 || servers[0].Name != c.want {
			t.Errorf("confName %q: servers %q, rejected %v, want %q", c.conf, names(servers), rejected, c.want)
		}
	}
	// links keep their own names
	servers, _, _ := ParseText(linkTrojan, "office")
	if len(servers) != 1 || servers[0].Name != "Trojan" {
		t.Errorf("links: %q", names(servers))
	}
}

func TestParseTextKeepsEachServersOwnText(t *testing.T) {
	servers, _, _ := ParseText(linkVLESS+"\n"+linkSS, "")
	if len(servers) != 2 || servers[0].Text != linkVLESS || servers[1].Text != linkSS {
		t.Fatalf("texts: %s", dump(servers))
	}
	// the stored text parses back to the same server
	for _, s := range servers {
		again, err := ParseLink(s.Text)
		if err != nil || !reflect.DeepEqual(again, s) {
			t.Errorf("%s does not parse back: %v", s.Name, err)
		}
	}
}

func wrap76(s string) string {
	var b strings.Builder
	for len(s) > 76 {
		b.WriteString(s[:76] + "\r\n")
		s = s[76:]
	}
	b.WriteString(s + "\r\n")
	return b.String()
}

func TestParseSubscription(t *testing.T) {
	list := linkVLESS + "\n" + linkTrojan + "\n" + linkSS + "\n"
	for len(list)%3 == 0 { // the encoding has padding to strip
		list += "\n"
	}
	std := base64.StdEncoding.EncodeToString([]byte(list))
	if !strings.HasSuffix(std, "=") {
		t.Fatal("the sample has no padding")
	}
	all := []string{"🇩🇪 Germany", "Trojan", "Outline"}
	cases := []struct {
		name    string
		body    string
		servers []string
		rejects []rej
	}{
		{"base64 std padded", std, all, nil},
		{"base64 std without padding", strings.TrimRight(std, "="), all, nil},
		{"base64 url without padding", base64.RawURLEncoding.EncodeToString([]byte(list)), all, nil},
		{"base64 url with padding", base64.URLEncoding.EncodeToString([]byte(list)), all, nil},
		{"base64 wrapped at 76 with crlf", wrap76(std), all, nil},
		{"base64 with a trailing newline", std + "\n", all, nil},
		{"plain lines with crlf", strings.ReplaceAll(list, "\n", "\r\n"), all, nil},
		{"plain lines with profile headers (Hiddify)", "#profile-title: base64:TXkgVlBO\n#profile-update-interval: 12\n\n" + list, all, nil},
		{"plain lines with rejects", linkTrojan + "\n" + linkPlugin + "\n" + "vless://" + uuid1 + "@example.com:443?type=xhttp\n", []string{"Trojan"},
			[]rej{{2, CodePlugin}, {3, CodeUnsupportedTransport}}},
		{"base64 with rejects keeps decoded line numbers", base64.StdEncoding.EncodeToString([]byte("\n" + linkTrojan + "\nbogus://x\n")), []string{"Trojan"},
			[]rej{{3, CodeUnsupportedProtocol}}},
		{"duplicates count once", list + list, all, nil},
		{"clash yaml", "proxies:\n  - name: a\n    type: ss\n    server: 192.0.2.1\n", nil, []rej{{0, CodeUnsupportedProtocol}}},
		{"clash yaml with addresses inside",
			"proxy-providers:\n  p:\n    type: http\n    url: https://sub.example.com/clash?token=x\n" +
				"proxies:\n  - {name: a, type: vless, server: 192.0.2.1, ws-opts: {path: /ws}}\nrule-providers:\n  r:\n    url: \"https://example.com/rules.yaml\"\n" +
				"  - https://example.com/list.txt\n#  - vless://" + uuid1 + "@example.com:443\n",
			nil, []rej{{0, CodeUnsupportedProtocol}}},
		{"sing-box json", `{"outbounds":[{"type":"vless","server":"192.0.2.1"}]}`, nil, []rej{{0, CodeUnsupportedProtocol}}},
		{"sing-box json with addresses inside, one line",
			`{"outbounds":[{"type":"vless","server":"192.0.2.1"}],"route":{"rule_set":[{"type":"remote","url":"https://example.com/geosite.srs"}]}}`,
			nil, []rej{{0, CodeUnsupportedProtocol}}},
		{"sing-box json with addresses inside, pretty",
			"{\n  \"route\": {\n    \"rule_set\": [\n      {\n        \"url\": \"https://example.com/geosite.srs\",\n        \"download_detour\": \"direct\"\n      }\n    ]\n  }\n}\n",
			nil, []rej{{0, CodeUnsupportedProtocol}}},
		{"wireguard file is not a subscription", wgConf, nil, []rej{{0, CodeUnsupportedProtocol}}},
		{"subscription address inside a subscription", "https://sub.example.com/next\n" + linkTrojan, []string{"Trojan"}, []rej{{1, CodeUnsupportedProtocol}}},
		{"empty", "", nil, nil},
		{"whitespace", " \r\n\t\n", nil, nil},
		{"too long", strings.Repeat("#", MaxSubscriptionLen+1), nil, []rej{{0, CodeTooLong}}},
		{"a line that is not utf-8", linkTrojan + "\n\xff\xfe\n" + linkSS, []string{"Trojan", "Outline"}, []rej{{2, CodeBadLink}}},
		{"a base64 line that is not utf-8", base64.StdEncoding.EncodeToString([]byte(linkTrojan + "\nvless://\xff\n" + linkSS)), []string{"Trojan", "Outline"},
			[]rej{{2, CodeBadLink}}},
		{"the same server under two names counts once", linkTrojan + "\n" + strings.Replace(linkTrojan, "#Trojan", "#Trojan | 3 GB left", 1), []string{"Trojan"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			servers, rejected := ParseSubscription([]byte(c.body))
			if got := names(servers); !reflect.DeepEqual(got, c.servers) {
				t.Errorf("servers %q, want %q", got, c.servers)
			}
			if got := rejs(rejected); !reflect.DeepEqual(got, c.rejects) {
				t.Errorf("rejected %v, want %v", got, c.rejects)
			}
		})
	}
}
