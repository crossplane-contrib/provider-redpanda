package clients

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func jwt(t *testing.T, exp time.Time) string {
	t.Helper()
	payload := fmt.Sprintf(`{"exp":%d}`, exp.Unix())
	return "h." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".s"
}

func TestParseTokenExpiry(t *testing.T) {
	want := time.Unix(time.Now().Add(time.Hour).Unix(), 0)

	cases := map[string]struct {
		token   string
		want    time.Time
		wantErr bool
	}{
		"valid JWT": {
			token: jwt(t, want),
			want:  want,
		},
		"wrong segment count": {
			token:   "h.p",
			wantErr: true,
		},
		"invalid base64 payload": {
			token:   "h.!!!.s",
			wantErr: true,
		},
		"invalid JSON payload": {
			token:   "h." + base64.RawURLEncoding.EncodeToString([]byte("not-json")) + ".s",
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := parseTokenExpiry(tc.token)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (time=%v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func callSetupFn(t *testing.T, cache *tokenCache, fetch tokenFetcher, id, secret string) (map[string]any, error) {
	t.Helper()
	extract := func(_ context.Context, _ client.Client, _ resource.Managed) (string, string, error) {
		return id, secret, nil
	}
	setup, err := buildSetupFn(logging.NewNopLogger(), cache, fetch, extract)(context.Background(), nil, nil)
	if err != nil {
		return nil, err
	}
	return setup.Configuration, nil
}

func TestBuildSetupFn_CachesAndExposesToken(t *testing.T) {
	calls := 0
	token := jwt(t, time.Now().Add(24*time.Hour))
	fetch := func(_ context.Context, _, _ string) (string, error) {
		calls++
		return token, nil
	}
	cache := &tokenCache{tokens: make(map[string]string)}

	for i := 0; i < 2; i++ {
		cfg, err := callSetupFn(t, cache, fetch, "id", "secret")
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if cfg[accessToken] != token {
			t.Errorf("call %d: wrong token", i)
		}
		if _, ok := cfg[clientID]; ok {
			t.Errorf("call %d: client_id leaked", i)
		}
	}
	if calls != 1 {
		t.Errorf("expected 1 fetch, got %d", calls)
	}
}

func TestBuildSetupFn_RefetchesNearExpiryToken(t *testing.T) {
	tokens := []string{
		jwt(t, time.Now().Add(30*time.Second)),
		jwt(t, time.Now().Add(24*time.Hour)),
	}
	calls := 0
	fetch := func(_ context.Context, _, _ string) (string, error) {
		tok := tokens[calls]
		calls++
		return tok, nil
	}
	cache := &tokenCache{tokens: make(map[string]string)}

	// First call: empty cache, fetcher returns a near-expiry token; trusted and cached.
	if _, err := callSetupFn(t, cache, fetch, "id", "secret"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	// Second call: cached token is within the expiry buffer, so cache.get errors and triggers a refetch.
	if _, err := callSetupFn(t, cache, fetch, "id", "secret"); err != nil {
		t.Fatalf("second call: %v", err)
	}
	// Third call: healthy cached token, no fetch.
	if _, err := callSetupFn(t, cache, fetch, "id", "secret"); err != nil {
		t.Fatalf("third call: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 fetches (near-expiry + good), got %d", calls)
	}
}
