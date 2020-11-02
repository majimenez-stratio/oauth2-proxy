package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
)

func TestNewSISProvider(t *testing.T) {
	g := NewWithT(t)

	// Test that defaults are set when calling for a new provider with nothing set
	providerData := NewSISProvider(&ProviderData{}).Data()
	g.Expect(providerData.ProviderName).To(Equal("SIS"))
	g.Expect(providerData.LoginURL.String()).To(Equal("https://sis/sso/oauth2.0/authorize"))
	g.Expect(providerData.RedeemURL.String()).To(Equal("https://sis/sso/oauth2.0/accessToken"))
	g.Expect(providerData.ProfileURL.String()).To(Equal("https://sis/sso/oauth2.0/profile"))
	g.Expect(providerData.ValidateURL.String()).To(Equal("https://sis/sso/oauth2.0/profile"))
	g.Expect(providerData.Scope).To(Equal("read"))
}

func TestSISProviderOverrides(t *testing.T) {
	p := NewSISProvider(
		&ProviderData{
			LoginURL: &url.URL{
				Scheme: "https",
				Host:   "example.com",
				Path:   "/login/oauth/authorize"},
			RedeemURL: &url.URL{
				Scheme: "https",
				Host:   "example.com",
				Path:   "/login/oauth/access_token"},
			ValidateURL: &url.URL{
				Scheme: "https",
				Host:   "api.example.com",
				Path:   "/"},
			Scope: "profile"})
	assert.NotEqual(t, nil, p)
	assert.Equal(t, "SIS", p.Data().ProviderName)
	assert.Equal(t, "https://example.com/login/oauth/authorize",
		p.Data().LoginURL.String())
	assert.Equal(t, "https://example.com/login/oauth/access_token",
		p.Data().RedeemURL.String())
	assert.Equal(t, "https://api.example.com/",
		p.Data().ValidateURL.String())
	assert.Equal(t, "profile", p.Data().Scope)
}

func TestSISProviderRedeem(t *testing.T) {
	b := testSISBackend(map[string]string{
		"/sso/oauth2.0/accessToken": "access_token=imaginary_access_token&expires=10000",
	})
	defer b.Close()

	bURL, _ := url.Parse(b.URL)
	p := testSISProvider(bURL)
	s, err := p.Redeem(context.Background(), "imaginary_redirect_url", "imaginary_code")
	assert.NoError(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, s.AccessToken, "imaginary_access_token")
	assert.NotNil(t, s.ExpiresOn)
}

func TestSISProviderEnrichSessionState(t *testing.T) {
	b := testSISBackend(map[string]string{
		"/sso/oauth2.0/profile": `{"id":"admin","attributes":[{"uid":"admin"},{"tenant":"NONE"},
{"roles":[]},{"groups":["admins","managers"]},{"cn":"admin"},{"mail":"admin@example.com"}]}`,
	})
	defer b.Close()

	bURL, _ := url.Parse(b.URL)
	p := testSISProvider(bURL)
	s := CreateAuthorizedSession()
	err := p.EnrichSessionState(context.Background(), s)
	assert.NoError(t, err)
	assert.Equal(t, s.User, "admin")
	assert.Equal(t, s.PreferredUsername, "admin")
	assert.Equal(t, s.Email, "admin@example.com")
	assert.Equal(t, s.Tenant, "NONE")
	assert.Equal(t, s.Groups, []string{"admins", "managers"})
}

func testSISBackend(payloads map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			payload, ok := payloads[r.URL.Path]
			if !ok {
				w.WriteHeader(404)
			} else {
				w.WriteHeader(200)
				w.Write([]byte(payload))
			}
		}))
}

func testSISProvider(rootURL *url.URL) *SISProvider {
	p := NewSISProvider(
		&ProviderData{
			ProviderName: "SIS",
			LoginURL:     &url.URL{},
			RedeemURL:    &url.URL{},
			ProfileURL:   &url.URL{},
			ValidateURL:  &url.URL{},
			Scope:        ""})
	p.Configure(rootURL)
	return p
}
