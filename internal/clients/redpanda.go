// Package clients provides Terraform setup functions for the Redpanda provider.
package clients

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"github.com/redpanda-data/terraform-provider-redpanda/redpanda"
	"github.com/redpanda-data/terraform-provider-redpanda/redpanda/cloud"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/upjet/v2/pkg/terraform"

	clusterv1beta1 "github.com/crossplane-contrib/provider-redpanda/apis/cluster/v1beta1"
	namespacedv1beta1 "github.com/crossplane-contrib/provider-redpanda/apis/namespaced/v1beta1"
)

const (
	clientID     = "client_id"
	clientSecret = "client_secret"
	accessToken  = "access_token"
)

const (
	// error messages
	errNoProviderConfig     = "no providerConfigRef provided"
	errGetProviderConfig    = "cannot get referenced ProviderConfig"
	errTrackUsage           = "cannot track ProviderConfig usage"
	errExtractCredentials   = "cannot extract credentials"
	errUnmarshalCredentials = "cannot unmarshal redpanda credentials as JSON"
	errMissingCredentials   = "credentials must contain client_id and client_secret"
)

type tokenFetcher func(ctx context.Context, clientID, clientSecret string) (string, error)

// tokenCache caches access tokens by clientID. get returns an error if the
// cached token is missing or within a minute of expiry, so callers refetch
// before the token can fail mid-request.
type tokenCache struct {
	mu     sync.RWMutex
	tokens map[string]string
}

func (c *tokenCache) get(id string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	token, ok := c.tokens[id]
	if !ok {
		return "", errors.New("no cached token")
	}
	exp, err := parseTokenExpiry(token)
	if err != nil {
		return "", err
	}
	if time.Until(exp) <= time.Minute {
		return "", errors.New("token within 1m of expiry")
	}

	return token, nil
}

func (c *tokenCache) set(id, token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens[id] = token
}

func parseTokenExpiry(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, errors.New("not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, errors.Wrap(err, "decode JWT payload")
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, errors.Wrap(err, "unmarshal JWT claims")
	}
	return time.Unix(claims.Exp, 0), nil
}

// credentialExtractor extracts the clientID and clientSecret for a managed resource.
// It is injectable so that buildSetupFn can be tested without a real K8s cluster.
type credentialExtractor func(ctx context.Context, c client.Client, mg resource.Managed) (id, secret string, err error)

// TerraformSetupBuilder builds a terraform.SetupFn that caches access tokens to
// avoid generating a new token on every reconcile loop.
func TerraformSetupBuilder(log logging.Logger) (terraform.SetupFn, error) {
	endpoint, err := cloud.EndpointForEnv("prod")
	if err != nil {
		return nil, errors.Wrap(err, "cannot resolve prod endpoint")
	}
	cache := &tokenCache{tokens: make(map[string]string)}
	fetch := func(ctx context.Context, id, secret string) (string, error) {
		return cloud.RequestToken(ctx, endpoint, id, secret)
	}
	return buildSetupFn(log, cache, fetch, extractCredentials), nil
}

// extractCredentials reads the clientID and clientSecret from the ProviderConfig
// referenced by the managed resource.
func extractCredentials(ctx context.Context, c client.Client, mg resource.Managed) (string, string, error) {
	pcSpec, err := resolveProviderConfig(ctx, c, mg)
	if err != nil {
		return "", "", errors.Wrap(err, "cannot resolve provider config")
	}
	data, err := resource.CommonCredentialExtractor(ctx, pcSpec.Credentials.Source, c, pcSpec.Credentials.CommonCredentialSelectors)
	if err != nil {
		return "", "", errors.Wrap(err, errExtractCredentials)
	}
	creds := map[string]string{}
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", "", errors.Wrap(err, errUnmarshalCredentials)
	}
	if creds[clientID] == "" || creds[clientSecret] == "" {
		return "", "", errors.New(errMissingCredentials)
	}
	return creds[clientID], creds[clientSecret], nil
}

// buildSetupFn constructs a terraform.SetupFn using injected dependencies for
// the token cache, fetcher, and credential extractor.
func buildSetupFn(log logging.Logger, cache *tokenCache, fetch tokenFetcher, extract credentialExtractor) terraform.SetupFn {
	return func(ctx context.Context, c client.Client, mg resource.Managed) (terraform.Setup, error) {
		ps := terraform.Setup{}

		id, secret, err := extract(ctx, c, mg)
		if err != nil {
			return ps, err
		}

		token, err := cache.get(id)
		if err != nil {
			log.Debug("fetching new access token", "clientID", id, "reason", err.Error())
			token, err = fetch(ctx, id, secret)
			if err != nil {
				return ps, errors.Wrap(err, "cannot fetch access token")
			}
			cache.set(id, token)
		}

		ps.Configuration = map[string]any{accessToken: token}
		ps.FrameworkProvider = redpanda.New(ctx, "prod", "v1.3.5")()
		return ps, nil
	}
}

func toSharedPCSpec(pc *clusterv1beta1.ProviderConfig) (*namespacedv1beta1.ProviderConfigSpec, error) {
	if pc == nil {
		return nil, nil
	}
	data, err := json.Marshal(pc.Spec)
	if err != nil {
		return nil, err
	}

	var mSpec namespacedv1beta1.ProviderConfigSpec
	err = json.Unmarshal(data, &mSpec)
	return &mSpec, err
}

func resolveProviderConfig(ctx context.Context, crClient client.Client, mg resource.Managed) (*namespacedv1beta1.ProviderConfigSpec, error) {
	switch managed := mg.(type) {
	case resource.LegacyManaged:
		return resolveLegacy(ctx, crClient, managed)
	case resource.ModernManaged:
		return resolveModern(ctx, crClient, managed)
	default:
		return nil, errors.New("resource is not a managed resource")
	}
}

func resolveLegacy(ctx context.Context, client client.Client, mg resource.LegacyManaged) (*namespacedv1beta1.ProviderConfigSpec, error) {
	configRef := mg.GetProviderConfigReference()
	if configRef == nil {
		return nil, errors.New(errNoProviderConfig)
	}
	pc := &clusterv1beta1.ProviderConfig{}
	if err := client.Get(ctx, types.NamespacedName{Name: configRef.Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetProviderConfig)
	}

	t := resource.NewLegacyProviderConfigUsageTracker(client, &clusterv1beta1.ProviderConfigUsage{})
	if err := t.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackUsage)
	}

	return toSharedPCSpec(pc)
}

func resolveModern(ctx context.Context, crClient client.Client, mg resource.ModernManaged) (*namespacedv1beta1.ProviderConfigSpec, error) {
	configRef := mg.GetProviderConfigReference()
	if configRef == nil {
		return nil, errors.New(errNoProviderConfig)
	}

	pcRuntimeObj, err := crClient.Scheme().New(namespacedv1beta1.SchemeGroupVersion.WithKind(configRef.Kind))
	if err != nil {
		return nil, errors.Wrap(err, "unknown GVK for ProviderConfig")
	}
	pcObj, ok := pcRuntimeObj.(client.Object)
	if !ok {
		// This indicates a programming error, types are not properly generated
		return nil, errors.New(" is not an Object")
	}

	// Namespace will be ignored if the PC is a cluster-scoped type
	if err := crClient.Get(ctx, types.NamespacedName{Name: configRef.Name, Namespace: mg.GetNamespace()}, pcObj); err != nil {
		return nil, errors.Wrap(err, errGetProviderConfig)
	}

	var pcSpec namespacedv1beta1.ProviderConfigSpec
	pcu := &namespacedv1beta1.ProviderConfigUsage{}
	switch pc := pcObj.(type) {
	case *namespacedv1beta1.ProviderConfig:
		pcSpec = pc.Spec
		if pcSpec.Credentials.SecretRef != nil {
			pcSpec.Credentials.SecretRef.Namespace = mg.GetNamespace()
		}
	case *namespacedv1beta1.ClusterProviderConfig:
		pcSpec = pc.Spec
	default:
		return nil, errors.New("unknown provider config type")
	}
	t := resource.NewProviderConfigUsageTracker(crClient, pcu)
	if err := t.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackUsage)
	}
	return &pcSpec, nil
}
