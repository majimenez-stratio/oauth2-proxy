package providers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/sessions"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/requests"
)

// SISProvider represents a Stratio Identity Server
type SISProvider struct {
	*ProviderData
}

var _ Provider = (*SISProvider)(nil)

const (
	sisProviderName = "SIS"
	sisDefaultScope = "read"
	sisDefaultHost  = "sis"
)

var (
	// Default Login URL for SIS.
	sisDefaultLoginURL = &url.URL{
		Scheme: "https",
		Host:   sisDefaultHost,
		Path:   "/sso/oauth2.0/authorize",
	}

	// Default Redeem URL for SIS.
	sisDefaultRedeemURL = &url.URL{
		Scheme: "https",
		Host:   sisDefaultHost,
		Path:   "/sso/oauth2.0/accessToken",
	}

	// Default Profile URL for SIS.
	sisDefaultProfileURL = &url.URL{
		Scheme: "https",
		Host:   sisDefaultHost,
		Path:   "/sso/oauth2.0/profile",
	}

	// Default Validate URL for SIS is same as Profile URL.
	sisDefaultValidateURL = sisDefaultProfileURL
)

// NewSISProvider initiates a new SISProvider
func NewSISProvider(p *ProviderData) *SISProvider {
	p.setProviderDefaults(providerDefaults{
		name:        sisProviderName,
		loginURL:    sisDefaultLoginURL,
		redeemURL:   sisDefaultRedeemURL,
		profileURL:  sisDefaultProfileURL,
		validateURL: sisDefaultValidateURL,
		scope:       sisDefaultScope,
	})

	return &SISProvider{p}
}

// Configure defaults the SISProvider configuration options
func (p *SISProvider) Configure(rootURL *url.URL) {
	p.LoginURL = &url.URL{
		Scheme: rootURL.Scheme,
		Host:   rootURL.Host,
		Path:   "/sso/oauth2.0/authorize",
	}

	// Default Redeem URL for SIS.
	p.RedeemURL = &url.URL{
		Scheme: rootURL.Scheme,
		Host:   rootURL.Host,
		Path:   "/sso/oauth2.0/accessToken",
	}

	// Default Profile URL for SIS.
	p.ProfileURL = &url.URL{
		Scheme: rootURL.Scheme,
		Host:   rootURL.Host,
		Path:   "/sso/oauth2.0/profile",
	}

	// Default Validate URL for SIS is same as Profile URL.
	p.ValidateURL = sisDefaultProfileURL
}

// Redeem provides a default implementation of the OAuth2 token redemption process
func (p *SISProvider) Redeem(ctx context.Context, redirectURL, code string) (s *sessions.SessionState, err error) {
	if code == "" {
		err = errors.New("missing code")
		return
	}
	clientSecret, err := p.GetClientSecret()
	if err != nil {
		return
	}

	params := url.Values{}
	params.Add("redirect_uri", redirectURL)
	params.Add("client_id", p.ClientID)
	params.Add("client_secret", clientSecret)
	params.Add("code", code)
	params.Add("grant_type", "authorization_code")
	if p.ProtectedResource != nil && p.ProtectedResource.String() != "" {
		params.Add("resource", p.ProtectedResource.String())
	}

	result := requests.New(p.RedeemURL.String()).
		WithContext(ctx).
		WithMethod("POST").
		WithBody(bytes.NewBufferString(params.Encode())).
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		Do()
	if result.Error() != nil {
		return nil, result.Error()
	}

	// blindly try json and x-www-form-urlencoded
	var jsonResponse struct {
		AccessToken string `json:"access_token"`
		ExpiresOn   int64  `json:"expires"`
	}

	err = result.UnmarshalInto(&jsonResponse)
	if err == nil {
		expires := time.Now().Add(time.Duration(jsonResponse.ExpiresOn) * time.Second).Truncate(time.Second)
		s = &sessions.SessionState{
			AccessToken: jsonResponse.AccessToken,
			ExpiresOn:   &expires,
		}
		return
	}

	var v url.Values
	v, err = url.ParseQuery(string(result.Body()))
	if err != nil {
		return
	}

	var expires time.Time

	if e := v.Get("expires"); e != "" {
		var i int
		i, err = strconv.Atoi(e)
		if err != nil {
			return
		}
		expires = time.Now().Add(time.Duration(i) * time.Second).Truncate(time.Second)
	} else {
		err = fmt.Errorf("no expiration found %s", result.Body())
	}

	if a := v.Get("access_token"); a != "" {
		created := time.Now()
		s = &sessions.SessionState{AccessToken: a, CreatedAt: &created, ExpiresOn: &expires}
	} else {
		err = fmt.Errorf("no access token found %s", result.Body())
	}
	return
}

func makeSISHeaders(accessToken string) http.Header {
	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	return header
}

// EnrichSessionState is called after Redeem to allow providers to enrich session fields
// such as User, Email, Groups with provider specific API calls.
func (p *SISProvider) EnrichSessionState(ctx context.Context, s *sessions.SessionState) error {
	json, err := requests.New(p.ProfileURL.String()).
		WithContext(ctx).
		WithHeaders(makeSISHeaders(s.AccessToken)).
		Do().
		UnmarshalJSON()
	if err != nil {
		return fmt.Errorf("error getting user info: %v", err)
	}

	attributes := json.GetPath("attributes")
	for i := range attributes.MustArray() {
		for k := range attributes.GetIndex(i).MustMap() {
			switch k {
			case "uid":
				s.User, err = attributes.GetIndex(i).Get("uid").String()
				if err != nil {
					fmt.Printf("Error unmarshalling %s: %v", k, err)
				}
			case "cn":
				s.PreferredUsername, err = attributes.GetIndex(i).Get("cn").String()
				if err != nil {
					fmt.Printf("Error unmarshalling %s: %v", k, err)
				}
			case "mail":
				s.Email, err = attributes.GetIndex(i).Get("mail").String()
				if err != nil {
					fmt.Printf("Error unmarshalling %s: %v", k, err)
				}
			case "tenant":
				s.Tenant, err = attributes.GetIndex(i).Get("tenant").String()
				if err != nil {
					fmt.Printf("Error unmarshalling %s: %v", k, err)
				}
			case "groups":
				s.Groups, err = attributes.GetIndex(i).Get("groups").StringArray()
				if err != nil {
					fmt.Printf("Error unmarshalling %s: %v", k, err)
				}
			}
		}
	}

	return nil
}
