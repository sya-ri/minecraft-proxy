package relay

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func reply(code int, body string) (*http.Response, error) {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestCreateRetriesWithOneVMNameAndRequestID(t *testing.T) {
	config := Config{Project: "test-project", Zone: "asia-northeast1-a", Network: "default", Domain: "mc.example.com"}
	session := identity{RequestID: requestID(), Nonce: "same-session", Deadline: time.Now().UTC().Truncate(time.Second).Add(4 * time.Hour)}
	calls := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodPost || r.URL.Query().Get("requestId") != session.RequestID {
			t.Fatalf("unexpected create request: %s %s", r.Method, r.URL)
		}
		var vm struct {
			Name       string
			Labels     map[string]string
			Scheduling map[string]string
			Disks      []struct{ AutoDelete bool }
		}
		if err := json.NewDecoder(r.Body).Decode(&vm); err != nil {
			t.Fatal(err)
		}
		if vm.Name != config.name() || vm.Labels["session"] != session.Nonce ||
			vm.Scheduling["instanceTerminationAction"] != "DELETE" ||
			vm.Scheduling["terminationTime"] != session.Deadline.Format(time.RFC3339) ||
			len(vm.Disks) != 1 || !vm.Disks[0].AutoDelete {
			t.Fatalf("incorrect VM identity or cleanup configuration: %+v", vm)
		}
		if calls == 1 {
			return nil, errors.New("response lost")
		}
		return reply(200, `{"status":"DONE"}`)
	})}
	if err := (compute{client, config}).create(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("sent %d requests, want 2", calls)
	}
}

func TestExtendChangesOnlyTheOwnedVMDeadline(t *testing.T) {
	config := Config{Project: "test-project", Zone: "asia-northeast1-a", Domain: "mc.example.com"}
	created := time.Now().UTC().Truncate(time.Second)
	before := created.Add(4 * time.Hour)
	for _, owner := range []bool{true, false} {
		if owner {
			t.Run("owner", func(t *testing.T) { testExtension(t, config, created, before, "local") })
		} else {
			t.Run("other session", func(t *testing.T) { testExtension(t, config, created, before, "another") })
		}
	}
}

func testExtension(t *testing.T, config Config, created, before time.Time, nonce string) {
	t.Helper()
	updates := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/instances/"+config.name()):
			vm := instance{
				ID: "42", Status: "RUNNING", CreationTimestamp: created,
				Labels: map[string]string{"managed-by": "minecraft-proxy", "relay": config.name(), "session": nonce},
			}
			vm.Scheduling.TerminationTime = before
			data, err := json.Marshal(vm)
			if err != nil {
				t.Fatal(err)
			}
			return reply(200, string(data))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/setScheduling"):
			updates++
			var scheduling map[string]string
			if err := json.NewDecoder(r.Body).Decode(&scheduling); err != nil {
				t.Fatal(err)
			}
			if scheduling["terminationTime"] != before.Add(4*time.Hour).Format(time.RFC3339) ||
				scheduling["instanceTerminationAction"] != "DELETE" || r.URL.Query().Get("requestId") == "" {
				t.Fatalf("unexpected schedule: %+v", scheduling)
			}
			return reply(200, `{"status":"DONE"}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
			return nil, nil
		}
	})}
	got, err := (compute{client, config}).extend(t.Context(), identity{Nonce: "local", InstanceID: "42"})
	if nonce != "local" {
		if err == nil || updates != 0 {
			t.Fatalf("attempted to extend a different session: %v, %d updates", err, updates)
		}
		return
	}
	if err != nil || updates != 1 || !got.Equal(before.Add(4*time.Hour)) {
		t.Fatalf("extend = %s, %v, %d updates", got, err, updates)
	}
}

func TestExistingDNSRequiresReplacementConsent(t *testing.T) {
	config := Config{Domain: "mc.example.com"}
	dns := cloudflare{config: config, client: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("name") == config.Domain {
			return reply(200, `{"result":[{"id":"other","type":"CNAME","name":"mc.example.com"}]}`)
		}
		return reply(200, `{"result":[{"id":"srv","type":"SRV","name":"_minecraft._tcp.mc.example.com"}]}`)
	})}}
	const root = "https://example.com/dns_records"
	if _, err := dns.ensureAvailable(t.Context(), root, identity{Nonce: "local"}, false); err == nil {
		t.Fatal("accepted replacing an existing DNS record without consent")
	}
	deletes, err := dns.ensureAvailable(t.Context(), root, identity{Nonce: "local"}, true)
	if err != nil || len(deletes) != 2 || deletes[0]["id"] != "other" || deletes[1]["id"] != "srv" {
		t.Fatalf("unexpected replacement plan: %+v, %v", deletes, err)
	}
}
