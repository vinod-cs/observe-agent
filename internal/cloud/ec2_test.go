// AGENTV1 FILE START: IMDSv2-only identity and redacted failure contracts.
package cloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEC2IMDSv2(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/latest/api/token" {
			if r.Method != "PUT" || r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds") != "60" {
				t.Error("missing IMDSv2 token request")
			}
			w.Write([]byte("test-token"))
			return
		}
		if r.Header.Get("X-aws-ec2-metadata-token") != "test-token" {
			t.Error("IMDSv1 fallback attempted")
		}
		w.Write([]byte(`{"accountId":"127696279140","region":"us-east-2","availabilityZone":"us-east-2a","instanceId":"i-0345d461c99a6da2f","instanceType":"c7i-flex.large"}`))
	}))
	defer server.Close()
	detector := NewEC2(time.Second)
	detector.base = server.URL
	evidence, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !evidence.Verified || evidence.InstanceID != "i-0345d461c99a6da2f" || evidence.Account != "127696279140" || evidence.ResourceID != "arn:aws:ec2:us-east-2:127696279140:instance/i-0345d461c99a6da2f" || requests != 2 {
		t.Fatalf("incorrect identity %+v", evidence)
	}
	if detector.client.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("IMDS must bypass proxies")
	}
}
func TestEC2NoDowngradeAndInvalidIdentity(t *testing.T) {
	for _, test := range []struct {
		name, body  string
		tokenStatus int
	}{{"denied", "", 403}, {"bad document", `{"instanceId":"hostname","accountId":"127696279140"}`, 200}, {"oversize", "", 200}} {
		t.Run(test.name, func(t *testing.T) {
			count := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count++
				if r.Method == "PUT" {
					w.WriteHeader(test.tokenStatus)
					w.Write([]byte("token"))
					return
				}
				if test.name == "oversize" {
					w.Write(make([]byte, 17000))
					return
				}
				w.Write([]byte(test.body))
			}))
			defer s.Close()
			d := NewEC2(time.Second)
			d.base = s.URL
			e, err := d.Detect(context.Background())
			if err == nil || e.Verified {
				t.Fatal("invalid evidence trusted")
			}
			if test.tokenStatus == 403 && count != 1 {
				t.Fatal("IMDSv1 fallback")
			}
		})
	}
}
func TestEC2Deadline(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer s.Close()
	d := NewEC2(20 * time.Millisecond)
	d.base = s.URL
	started := time.Now()
	if _, e := d.Detect(context.Background()); e == nil {
		t.Fatal("timeout accepted")
	}
	if time.Since(started) > time.Second {
		t.Fatal("unbounded metadata request")
	}
}

// AGENTV1 FILE END
