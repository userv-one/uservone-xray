package conf_test

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/xtls/xray-core/infra/conf"
	confserial "github.com/xtls/xray-core/infra/conf/serial"
)

func readExampleFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func buildExampleFixture(t *testing.T, name string) {
	t.Helper()
	if _, err := confserial.LoadJSONConfig(strings.NewReader(string(readExampleFixture(t, name)))); err != nil {
		t.Fatalf("%s failed to build: %v", name, err)
	}
}

type exampleSockopt struct {
	TCPKeepAliveIdle     int32 `json:"tcpKeepAliveIdle"`
	TCPKeepAliveInterval int32 `json:"tcpKeepAliveInterval"`
	TCPKeepAliveCount    int32 `json:"tcpKeepAliveCount"`
}

func TestServerExample(t *testing.T) {
	buildExampleFixture(t, "uservone.server.json")
	var config struct {
		Inbounds []struct {
			Tag            string `json:"tag"`
			Port           int    `json:"port"`
			Protocol       string `json:"protocol"`
			Settings       json.RawMessage
			StreamSettings struct {
				Sockopt exampleSockopt `json:"sockopt"`
			} `json:"streamSettings"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(readExampleFixture(t, "uservone.server.json"), &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Inbounds) != 3 {
		t.Fatalf("got %d inbounds, want 3", len(config.Inbounds))
	}
	inbounds := make(map[string]int, len(config.Inbounds))
	for _, inbound := range config.Inbounds {
		inbounds[inbound.Tag] = inbound.Port
		if inbound.StreamSettings.Sockopt != (exampleSockopt{60, 15, 5}) {
			t.Errorf("%s has unexpected keepalive settings: %+v", inbound.Tag, inbound.StreamSettings.Sockopt)
		}
	}
	if inbounds["ss-in"] != 8388 || inbounds["socks-in"] != 1080 {
		t.Fatalf("unexpected inbound ports: %+v", inbounds)
	}

	var shadowsocks struct {
		Method   string `json:"method"`
		Password string `json:"password"`
		Network  string `json:"network"`
	}
	if err := json.Unmarshal(config.Inbounds[1].Settings, &shadowsocks); err != nil {
		t.Fatal(err)
	}
	if config.Inbounds[1].Tag != "ss-in" || shadowsocks.Method != "2022-blake3-aes-128-gcm" || shadowsocks.Password == "" || shadowsocks.Network != "tcp,udp" {
		t.Fatalf("unexpected Shadowsocks configuration: %+v", shadowsocks)
	}

	var socks struct {
		Auth     string `json:"auth"`
		UDP      bool   `json:"udp"`
		Accounts []struct {
			Username string `json:"user"`
			Password string `json:"pass"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(config.Inbounds[2].Settings, &socks); err != nil {
		t.Fatal(err)
	}
	if config.Inbounds[2].Tag != "socks-in" || socks.Auth != AuthMethodUserPass || !socks.UDP || len(socks.Accounts) != 1 {
		t.Fatalf("unexpected SOCKS configuration: %+v", socks)
	}
	if socks.Accounts[0].Username != "f1474e069739465d" || socks.Accounts[0].Password != "0739beac2d74457e" {
		t.Fatal("SOCKS fixture no longer matches deriveSocks5Credentials")
	}
}

func TestVLESSRealityClientExample(t *testing.T) {
	buildExampleFixture(t, "uservone.vless-reality.client.json")
	var config struct {
		Outbounds []struct {
			Protocol string `json:"protocol"`
			Settings struct {
				Vnext []struct {
					Users []struct {
						Flow       string `json:"flow"`
						Encryption string `json:"encryption"`
					} `json:"users"`
				} `json:"vnext"`
			} `json:"settings"`
			StreamSettings struct {
				Security        string `json:"security"`
				RealitySettings struct {
					ServerName  string `json:"serverName"`
					Fingerprint string `json:"fingerprint"`
					PublicKey   string `json:"publicKey"`
					ShortID     string `json:"shortId"`
				} `json:"realitySettings"`
				Sockopt exampleSockopt `json:"sockopt"`
			} `json:"streamSettings"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal(readExampleFixture(t, "uservone.vless-reality.client.json"), &config); err != nil {
		t.Fatal(err)
	}
	outbound := config.Outbounds[0]
	if outbound.Protocol != "vless" || outbound.Settings.Vnext[0].Users[0].Flow != "xtls-rprx-vision" || outbound.Settings.Vnext[0].Users[0].Encryption != "none" {
		t.Fatal("VLESS Vision settings are not preserved")
	}
	reality := outbound.StreamSettings.RealitySettings
	if outbound.StreamSettings.Security != "reality" || reality.ServerName != "www.cloudflare.com" || reality.Fingerprint != "chrome" || reality.PublicKey == "" || reality.ShortID != "0123456789abcdef" {
		t.Fatalf("unexpected REALITY settings: %+v", reality)
	}
	if outbound.StreamSettings.Sockopt != (exampleSockopt{60, 15, 5}) {
		t.Fatalf("unexpected client keepalive settings: %+v", outbound.StreamSettings.Sockopt)
	}
}

func TestSubscriptionFixtures(t *testing.T) {
	vlessURL, err := url.Parse(strings.TrimSpace(string(readExampleFixture(t, "uservone.vless-reality.url"))))
	if err != nil {
		t.Fatal(err)
	}
	query := vlessURL.Query()
	for key, expected := range map[string]string{
		"flow":           "xtls-rprx-vision",
		"packetEncoding": "xudp",
		"pbk":            "ZKZ_J-hAQH3Wct75N2w2RVNRl4fpnWbQbK7XJC-QLyw",
		"sid":            "0123456789abcdef",
		"sni":            "www.cloudflare.com",
		"fp":             "chrome",
		"spx":            "/",
	} {
		if query.Get(key) != expected {
			t.Errorf("VLESS URI %s is %q, want %q", key, query.Get(key), expected)
		}
	}

	ssURL, err := url.Parse(strings.TrimSpace(string(readExampleFixture(t, "uservone.shadowsocks.url"))))
	if err != nil {
		t.Fatal(err)
	}
	if ssURL.Hostname() != "127.0.0.1" || ssURL.Port() != "8388" || ssURL.Fragment != "uservone" {
		t.Fatalf("unexpected SIP002 endpoint: %s", ssURL)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(ssURL.User.String())
	if err != nil {
		t.Fatalf("SIP002 userinfo is not base64url: %v", err)
	}
	if string(decoded) != "2022-blake3-aes-128-gcm:BmtPW3egtT7pVAK9wpRzlg==" {
		t.Fatalf("unexpected SIP002 credentials encoding: %s", decoded)
	}
}

func TestAEAD2022SubscriptionFixture(t *testing.T) {
	raw := strings.TrimSpace(string(readExampleFixture(t, "uservone.shadowsocks-aead2022.url")))
	uri, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if uri.User.Username() != "2022-blake3-aes-128-gcm" {
		t.Fatalf("unexpected AEAD-2022 method: %q", uri.User.Username())
	}
	password, ok := uri.User.Password()
	if !ok || password != "BmtPW3egtT7pVAK9wpRzlg==" {
		t.Fatal("AEAD-2022 password was not correctly percent encoded")
	}
	userinfo := strings.Split(strings.TrimPrefix(raw, "ss://"), "@")[0]
	if userinfo != "2022-blake3-aes-128-gcm:BmtPW3egtT7pVAK9wpRzlg%3D%3D" {
		t.Fatalf("AEAD-2022 userinfo must remain unencoded except percent escapes: %q", userinfo)
	}
	if _, err := base64.RawURLEncoding.DecodeString(userinfo); err == nil {
		t.Fatal("AEAD-2022 userinfo must not be base64url encoded")
	}
}
