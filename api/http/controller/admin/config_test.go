package admin

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/gin-gonic/gin"

	"rustdesk-server/api/config"
	"rustdesk-server/api/global"
	"rustdesk-server/api/service"
)

// AllConfig is the admin-panel contract. Feature 2 (issue #82) adds the
// public_* keys additively: they are present whether or not the env-configured
// values are set, and the operational id_server/relay_server/api_server keys
// keep their internal-address semantics unchanged.
func TestAllConfigExposesPublicAddressKeysAdditively(t *testing.T) {
	previous := global.Config
	t.Cleanup(func() { global.Config = previous })

	for name, rustdesk := range map[string]config.Rustdesk{
		"public set": {
			IdServer:          "localhost:21116",
			RelayServer:       "localhost:21117",
			ApiServer:         "http://localhost:21114",
			PublicIdServer:    "relay.example.com:21116",
			PublicRelayServer: "relay.example.com:21117",
			PublicApiServer:   "https://relay.example.com:21114",
			Key:               "test-key",
			WsHost:            "ws://localhost:21118",
		},
		"public absent": {
			IdServer:    "localhost:21116",
			RelayServer: "localhost:21117",
			ApiServer:   "http://localhost:21114",
			Key:         "test-key",
			WsHost:      "ws://localhost:21118",
		},
	} {
		t.Run(name, func(t *testing.T) {
			global.Config = config.Config{Rustdesk: rustdesk}

			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			(&Config{}).AllConfig(context)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
			}

			var payload struct {
				Data map[string]interface{} `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatalf("invalid response: %v", err)
			}
			got := payload.Data

			// Additive contract: always present, empty string when unset.
			for key, want := range map[string]string{
				"public_id_server":    rustdesk.PublicIdServer,
				"public_relay_server": rustdesk.PublicRelayServer,
				"public_api_server":   rustdesk.PublicApiServer,
			} {
				value, ok := got[key].(string)
				if !ok {
					t.Fatalf("data.%s missing or not a string: %#v", key, got[key])
				}
				if value != want {
					t.Fatalf("data.%s = %q, want %q", key, value, want)
				}
			}

			// Operational keys keep their exact internal-address values.
			for key, want := range map[string]string{
				"id_server":    rustdesk.IdServer,
				"relay_server": rustdesk.RelayServer,
				"api_server":   rustdesk.ApiServer,
				"key":          rustdesk.Key,
				"ws_host":      rustdesk.WsHost,
			} {
				if got[key] != want {
					t.Fatalf("data.%s = %#v, want %q", key, got[key], want)
				}
			}
		})
	}
}

// ServerAddresses resolves through the injected enumerator, so this test never
// touches the live network.
func TestServerAddressesUsesInjectedEnumerator(t *testing.T) {
	synthetic := []service.NetworkInterface{
		{Name: "lo", Flags: net.FlagUp | net.FlagLoopback, Addrs: []netip.Addr{netip.MustParseAddr("127.0.0.1")}},
		{Name: "eth0", Flags: net.FlagUp, Addrs: []netip.Addr{netip.MustParseAddr("192.168.1.66")}},
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	co := &Config{networkEnumerator: func() ([]service.NetworkInterface, error) {
		return synthetic, nil
	}}
	co.ServerAddresses(context)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}

	var payload struct {
		Data struct {
			Addresses []struct {
				Interface string `json:"interface"`
				Address   string `json:"address"`
				Family    string `json:"family"`
			} `json:"addresses"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if len(payload.Data.Addresses) != 1 {
		t.Fatalf("addresses = %+v, want 1 entry", payload.Data.Addresses)
	}
	entry := payload.Data.Addresses[0]
	if entry.Interface != "eth0" || entry.Address != "192.168.1.66" || entry.Family != "ipv4" {
		t.Fatalf("entry = %+v, want eth0/192.168.1.66/ipv4", entry)
	}
}
