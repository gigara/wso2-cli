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

package contexts_test

import (
	"strings"
	"testing"

	"github.com/wso2/wso2-cli/internal/contexts"
)

// cloudIssuer is the URL a provider is served from in the
// documents below, and example-idp the identifier it names.
const cloudIssuer = "https://idp.example.test"

// cloudDocument is a current document whose login declares issuerID, or no
// issuerId member at all when issuerID is nil.
func cloudDocument(kind string, issuerID *string) string {
	member := ""
	if issuerID != nil {
		member = `, "issuerId": "` + *issuerID + `"`
	}
	credential := `"credentialRef": "cloud-login",`
	login := `"kind": "` + kind + `", "issuer": "` + cloudIssuer + `", "clientId": "wso2-cli", "product": "ref"` + member
	if kind == contexts.KindClientCredentials {
		credential = ""
		login = `"kind": "` + kind + `", "issuer": "` + cloudIssuer + `", "clientId": "wso2-cli", ` +
			`"clientSecretVariable": "WSO2_SECRET"` + member
	}
	if kind == contexts.KindPAT {
		login = `"kind": "pat", "product": "ref"` + member
	}
	return `{
  "schemaVersion": 4,
  "defaultContext": "cloud",
  "contexts": [{
    "name": "cloud", "type": "cloud", ` + credential + `
    "login": {` + login + `},
    "products": {"ref": {"url": "https://api.example.test", "audience": "ref", "scopes": ["ref:read"]}}
  }]
}`
}

func text(value string) *string { return &value }

func TestAnIssuerIDIsAcceptedOnEveryKindThatLogsInThroughAnIssuer(t *testing.T) {
	for _, kind := range []string{contexts.KindOAuthBrowser, contexts.KindOAuthDevice, contexts.KindClientCredentials} {
		t.Run(kind, func(t *testing.T) {
			document, err := contexts.Decode([]byte(cloudDocument(kind, text("example-idp"))))
			if err != nil {
				t.Fatalf("an issuerId on the %s kind was refused: %v", kind, err)
			}
			login := document.Contexts[0].Login
			if login.IssuerID != "example-idp" || login.Issuer != cloudIssuer {
				t.Fatalf("login decoded as %+v", login)
			}
		})
	}
}

func TestAnIssuerIDIsValidated(t *testing.T) {
	for name, testCase := range map[string]struct {
		kind     string
		issuerID string
		code     string
	}{
		"a plain identifier":           {contexts.KindOAuthBrowser, "example-idp", ""},
		"an https identifier":          {contexts.KindOAuthBrowser, "https://other.example.test", ""},
		"a loopback http identifier":   {contexts.KindOAuthBrowser, "http://localhost:8090", ""},
		"an empty identifier":          {contexts.KindOAuthBrowser, "", "contexts.document_malformed"},
		"an identifier with a space":   {contexts.KindOAuthBrowser, "platform idp", "contexts.document_malformed"},
		"an identifier padded":         {contexts.KindOAuthBrowser, " example-idp", "contexts.document_malformed"},
		"a urn identifier":             {contexts.KindOAuthBrowser, "urn:example:idp", ""},
		"a host and port identifier":   {contexts.KindOAuthBrowser, "idp:8443", ""},
		"the issuer itself":            {contexts.KindOAuthBrowser, cloudIssuer, "contexts.document_malformed"},
		"the issuer with a slash":      {contexts.KindOAuthBrowser, cloudIssuer + "/", "contexts.document_malformed"},
		"a plaintext identifier":       {contexts.KindOAuthBrowser, "http://idp.example.test", "contexts.document_malformed"},
		"an identifier on a pat login": {contexts.KindPAT, "example-idp", "contexts.document_malformed"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := contexts.Decode([]byte(cloudDocument(testCase.kind, text(testCase.issuerID))))
			assertProblemCode(t, err, testCase.code)
			if err != nil && !strings.Contains(err.Error(), "issuer identifier") {
				t.Errorf("refused for another reason than the issuer identifier: %v", err)
			}
			if err != nil && testCase.issuerID != "" && strings.Contains(err.Error(), testCase.issuerID) {
				t.Errorf("the refusal echoes the configured identifier: %q", err.Error())
			}
		})
	}
}

// The declared identifier travels with every access reached at the login
// issuer, and with none at a product grant's own issuer.
func TestTheIssuerIDTravelsOnlyWithAccessAtTheLoginIssuer(t *testing.T) {
	account := contexts.Account{
		Name: "cloud", Type: "cloud", LoginProduct: "ref",
		Auth: contexts.AccountAuth{Kind: contexts.KindOAuthBrowser, Issuer: cloudIssuer, IssuerID: "example-idp",
			ClientID: "wso2-cli", CredentialRef: "cloud"},
		Products: map[string]contexts.Product{
			"ref": {Endpoint: "https://api.example.test", Audience: "ref", Scopes: []string{"ref:read"},
				Gateway: &contexts.Gateway{Endpoint: "https://gw.example.test", Audience: "gw"}},
			"other": {Endpoint: "https://other.example.test", Audience: "other", Scopes: []string{"other:read"}},
			"api": {Endpoint: "https://api2.example.test", Audience: "api2",
				Grant: &contexts.Grant{Kind: contexts.GrantExchange}},
			"apim": {Endpoint: "https://apim.example.test", Audience: "apim",
				Grant: &contexts.Grant{Kind: contexts.GrantFederated,
					Issuer: "https://apim.example.test/oauth2/token", ClientID: "apim-cli"}},
		},
	}
	if login := account.LoginAccess(); login.IssuerID != "example-idp" {
		t.Errorf("login access = %+v, want the declared identifier", login)
	}
	for _, key := range []string{"ref", contexts.GatewayKey("ref"), "other", "api"} {
		if access, _ := account.Access(key); access.IssuerID != "example-idp" {
			t.Errorf("%s access = %+v, want the declared identifier", key, access)
		}
	}
	if access, _ := account.Access("apim"); access.IssuerID != "" {
		t.Errorf("federated access = %+v, want no identifier at the grant's own issuer", access)
	}
}

func TestAPlaintextIssuerIDIsRefusedForItsTransport(t *testing.T) {
	_, err := contexts.Decode([]byte(cloudDocument(contexts.KindOAuthBrowser,
		text("http://idp.example.test"))))
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("a plaintext issuerId was not refused for its transport: %v", err)
	}
}

func TestAnIssuerIDIsRefusedOnAProductGrant(t *testing.T) {
	document := strings.Replace(cloudDocument(contexts.KindOAuthBrowser, nil),
		`"scopes": ["ref:read"]}}`,
		`"scopes": ["ref:read"]}, "apim": {"url": "https://apim.example.test", "audience": "apim", "grant": `+
			`{"kind": "federated", "issuer": "https://apim.example.test/oauth2/token", `+
			`"clientId": "apim-cli"}}}`, 1)
	if _, err := contexts.Decode([]byte(document)); err != nil {
		t.Fatalf("the grant document is refused without an issuerId: %v", err)
	}
	_, err := contexts.Decode([]byte(strings.Replace(document, `"clientId": "apim-cli"`,
		`"clientId": "apim-cli", "issuerId": "apim-idp"`, 1)))
	assertProblemCode(t, err, "contexts.document_malformed")
	if !strings.Contains(err.Error(), "issuer identifier") {
		t.Fatalf("refused for another reason than the issuer identifier: %v", err)
	}
}

func TestADocumentWithoutAnIssuerIDLoadsAndWritesUnchanged(t *testing.T) {
	document, err := contexts.Decode([]byte(cloudDocument(contexts.KindOAuthBrowser, nil)))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if document.Contexts[0].Login.IssuerID != "" {
		t.Fatalf("a login without issuerId decoded one: %+v", document.Contexts[0].Login)
	}
	written, err := document.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if strings.Contains(string(written), "issuerId") {
		t.Fatalf("a login without issuerId was written with one:\n%s", written)
	}
}

func TestADeclaredIssuerIDIsWrittenBack(t *testing.T) {
	document, err := contexts.Decode([]byte(cloudDocument(contexts.KindOAuthBrowser, text("example-idp"))))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	written, err := document.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.Contains(string(written), "issuerId: example-idp") {
		t.Fatalf("the declared issuerId was not written back:\n%s", written)
	}
}
