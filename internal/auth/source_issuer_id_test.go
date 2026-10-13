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

package auth_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/wso2/wso2-cli/internal/auth"
	"github.com/wso2/wso2-cli/internal/auth/fakeissuer"
)

// cloudIssuerID is the identifier a provider names in place of the
// URL it is served from.
const cloudIssuerID = "example-idp"

func TestALoginSessionAtAProviderNamingTheDeclaredIssuerIDRenews(t *testing.T) {
	deployment := seedBrowserSession(t, fakeissuer.Options{IssuerIdentifier: cloudIssuerID})
	broker := deployment.broker(t)
	broker.Selection.Identity.Auth.IssuerID = cloudIssuerID

	grant, err := broker.Acquire(declaredRequest())
	if err != nil {
		t.Fatalf("Acquire returned %v", err)
	}
	if active, _, _ := deployment.issuer.Introspect(t, grant.Token); !active {
		t.Fatal("the renewal granted a token the provider did not mint")
	}
	// The session stays bound to login.issuer, the URL, not the identifier.
	if stored := deployment.storedSession(t); stored.Issuer != deployment.issuer.URL {
		t.Fatalf("the renewed session names the issuer %q, want %q", stored.Issuer, deployment.issuer.URL)
	}
}

func TestALoginSessionAtAProviderNamingAnUndeclaredIssuerIDIsRefusedByName(t *testing.T) {
	deployment := seedBrowserSession(t, fakeissuer.Options{IssuerIdentifier: cloudIssuerID})

	_, err := deployment.broker(t).Acquire(declaredRequest())
	var denial auth.Denial
	if !errors.As(err, &denial) {
		t.Fatalf("a provider naming another issuer was not refused: %v", err)
	}
	if denial.Problem.Code != "auth.discovery_failed" ||
		!strings.Contains(denial.Problem.Message, `"example-idp"`) ||
		!strings.Contains(denial.Problem.Recovery, "login.issuerId") {
		t.Fatalf("refusal = %+v, want the issuer mismatch naming example-idp", denial.Problem)
	}
}

func TestAnExchangeFromALoginSessionWithAnIssuerIDIsGranted(t *testing.T) {
	deployment := seedExchangeDeployment(t, fakeissuer.Options{IssuerIdentifier: cloudIssuerID})
	broker := deployment.broker(t)
	broker.Selection.Identity.Auth.IssuerID = cloudIssuerID

	grant, err := broker.Acquire(declaredRequest())
	if err != nil {
		t.Fatalf("Acquire returned %v", err)
	}
	if grant.Token == "" {
		t.Fatal("the exchange granted no token")
	}
}
