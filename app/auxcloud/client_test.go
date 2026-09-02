package auxcloud

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/service/models"
)

func TestClientLoginDiscoveryAndControl(t *testing.T) {
	var loginCount int
	cookie := base64.StdEncoding.EncodeToString([]byte(`{"terminalid":"term-1","aeskey":"aes-key"}`))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/account/login":
			loginCount++
			body, _ := io.ReadAll(r.Body)
			ts := r.Header.Get("timestamp")
			if ts == "" || r.Header.Get("token") == "" {
				t.Errorf("missing login headers")
			}
			decrypted, err := decryptAESCBCZeroPad(timestampKey(mustInt64(ts)), aesIV, body)
			if err != nil {
				t.Errorf("decrypt login: %v", err)
			}
			if !strings.Contains(string(decrypted), `"email":"user@example.com"`) {
				t.Errorf("login payload = %s", decrypted)
			}
			if strings.Contains(string(decrypted), "plain-password") {
				t.Errorf("password was not hashed")
			}
			writeJSON(w, map[string]any{"status": 0, "loginsession": "session-1", "userid": "user-1"})
		case r.URL.Path == "/appsync/group/member/getfamilylist":
			if r.Header.Get("loginsession") != "session-1" {
				t.Errorf("missing session header")
			}
			writeJSON(w, map[string]any{
				"status": 0,
				"data":   map[string]any{"familyList": []map[string]string{{"familyid": "fam-1", "name": "Home"}}},
			})
		case strings.HasPrefix(r.URL.Path, "/appsync/group/dev/query"):
			writeJSON(w, map[string]any{
				"status": 0,
				"data": map[string]any{
					"endpoints": []map[string]string{{
						"endpointId":   "ep-1",
						"friendlyName": "Living Room AC",
						"productId":    "pid-1",
						"mac":          "34:ea:34:5b:0f:d4",
						"devSession":   "dev-session",
						"cookie":       cookie,
					}},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/device/control/v2/sdkcontrol"):
			if r.URL.Query().Get("license") == "" {
				t.Errorf("missing license query")
			}
			raw, _ := io.ReadAll(r.Body)
			var req controlRequest
			if err := json.Unmarshal(raw, &req); err != nil {
				t.Errorf("control json: %v", err)
			}
			if req.Directive.Payload.Act == "get" {
				writeJSON(w, map[string]any{
					"event": map[string]any{
						"header":  map[string]string{"name": "Response"},
						"payload": map[string]string{"data": `{"params":["pwr","ac_mode","temp","envtemp","ac_mark","ac_vdir","scrdisp"],"vals":[[{"val":1}],[{"val":0}],[{"val":240}],[{"val":250}],[{"val":1}],[{"val":0}],[{"val":1}]]}`},
					},
				})
				return
			}
			writeJSON(w, map[string]any{
				"event": map[string]any{
					"header":  map[string]string{"name": "Response"},
					"payload": map[string]string{"data": `{"params":["pwr"],"vals":[[{"val":0}]]}`},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{Email: "user@example.com", Password: "plain-password", Region: "eu", HTTP: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	client.baseURL = server.URL
	client.requestGap = 0

	if err = client.Login(t.Context()); err != nil {
		t.Fatalf("login: %v", err)
	}

	devices, err := client.DiscoverDevices(t.Context())
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(devices) != 1 || devices[0].Mac != "34ea345b0fd4" {
		t.Fatalf("devices = %+v", devices)
	}

	backend := NewBackend(client)
	if err = backend.Authenticate(t.Context(), "34ea345b0fd4"); err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	state, err := backend.ReadState(t.Context(), "34ea345b0fd4")
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if state.Status.Mode != "cool" || state.Status.Temperature != 24 || state.AmbientTemp == nil || *state.AmbientTemp != 25 {
		t.Fatalf("state = %+v", state.Status)
	}

	mode := "off"
	if err = backend.ApplyState(t.Context(), "34ea345b0fd4", &models.UpdateDeviceStatesInput{Mode: &mode}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if loginCount == 0 {
		t.Fatal("expected login")
	}
}

func TestClientReloginOnUnauthorized(t *testing.T) {
	cookie := base64.StdEncoding.EncodeToString([]byte(`{"terminalid":"term-1","aeskey":"aes-key"}`))
	var logins int
	var gets int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/account/login":
			logins++
			writeJSON(w, map[string]any{"status": 0, "loginsession": "session", "userid": "user"})
		case "/appsync/group/member/getfamilylist":
			writeJSON(w, map[string]any{"status": 0, "data": map[string]any{"familyList": []map[string]string{{"familyid": "fam-1"}}}})
		default:
			if strings.HasPrefix(r.URL.Path, "/appsync/group/dev/query") {
				writeJSON(w, map[string]any{"status": 0, "data": map[string]any{"endpoints": []map[string]string{{
					"endpointId": "ep-1", "friendlyName": "AC", "productId": "pid-1",
					"mac": "34ea345b0fd4", "devSession": "dev-session", "cookie": cookie,
				}}}})
				return
			}
			gets++
			if gets == 1 {
				writeJSON(w, map[string]any{
					"event": map[string]any{
						"header":  map[string]string{"name": "ErrorResponse"},
						"payload": map[string]string{"type": "UNAUTHORIZED", "message": "login session expired"},
					},
				})
				return
			}
			writeJSON(w, map[string]any{
				"event": map[string]any{
					"header":  map[string]string{"name": "Response"},
					"payload": map[string]string{"data": `{"params":["pwr"],"vals":[[{"val":1}]]}`},
				},
			})
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{Email: "user@example.com", Password: "secret", Region: "eu", HTTP: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	client.baseURL = server.URL
	client.requestGap = 0
	client.replaceDevices([]Device{{
		EndpointID: "ep-1", ProductID: "pid-1", Mac: "34ea345b0fd4",
		DevSession: "dev-session", Cookie: cookie,
	}})
	client.loginSession = "expired"
	client.userID = "user"

	if _, err = client.GetDeviceParams(t.Context(), "34ea345b0fd4", nil); err != nil {
		t.Fatalf("retry after relogin: %v", err)
	}
	if logins == 0 {
		t.Fatal("expected relogin")
	}
}

func TestRedactSecret(t *testing.T) {
	if redactSecret("password") != "[redacted]" {
		t.Fatal("secret was not redacted")
	}
	if redactSecret("") != "" {
		t.Fatal("empty secret should stay empty")
	}
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func mustInt64(v string) int64 {
	var n int64
	for _, r := range v {
		n = n*10 + int64(r-'0')
	}
	return n
}
