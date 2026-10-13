// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package auth

import (
	"context"
	"errors"
	"net/http"

	oidc "github.com/coreos/go-oidc/v3/oidc"

	"github.com/wso2/wso2-cli/internal/auth/oidcdiscovery"
	"github.com/wso2/wso2-cli/internal/auth/trustedhttp"
	"github.com/wso2/wso2-cli/sdk/problem"
)

// tokenEndpoint resolves an issuer's token endpoint through OpenID discovery.
//
// The endpoint is discovered rather than derived, because the shell's whole
// claim to work against both Asgardeo and a self-hosted Identity Server is that
// it reads the deployment's own configuration instead of assuming a URL shape.
// Discovery also validates that the document belongs to the issuer it was
// fetched from, so a redirected host cannot substitute its own token endpoint.
//
// issuerID is the issuer identifier the context declares the provider at
// issuer names, empty when it names issuer itself or issuer is a product
// grant's own.
func tokenEndpoint(ctx context.Context, client *http.Client, issuer, issuerID string) (string, error) {
	provider, err := discover(ctx, client, issuer, issuerID)
	if err != nil {
		return "", err
	}
	endpoint := provider.Endpoint().TokenURL
	if endpoint == "" {
		return "", Denial{Problem: oidcdiscovery.Unreadable()}
	}
	return endpoint, nil
}

// ProbeIssuer reads an issuer's OpenID configuration and reports nothing
// else about it. It is the check wso2 doctor --online runs, and it refuses
// exactly the way a product command's own discovery would, so the two never
// disagree about a deployment. issuerID is as on tokenEndpoint.
func ProbeIssuer(ctx context.Context, client *http.Client, issuer, issuerID string) error {
	_, err := discover(ctx, client, issuer, issuerID)
	return err
}

// discover fetches the issuer's configuration through oidcdiscovery.Discover,
// which classifies every failure into a problem without the provider's own
// words: that error may quote the request that produced it, and the shell
// renders problems verbatim.
func discover(ctx context.Context, client *http.Client, issuer, issuerID string) (*oidc.Provider, error) {
	provider, err := oidcdiscovery.Discover(oidc.ClientContext(ctx, trustedhttp.Client(client)), issuer, issuerID)
	if err != nil {
		var refused problem.Problem
		if errors.As(err, &refused) {
			return nil, Denial{Problem: refused}
		}
		return nil, Denial{Problem: oidcdiscovery.Unreadable()}
	}
	return provider, nil
}
