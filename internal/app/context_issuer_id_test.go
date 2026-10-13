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

package app_test

import (
	"os"
	"strings"
	"testing"

	"github.com/wso2/wso2-cli/internal/contexts"
	"github.com/wso2/wso2-cli/internal/exit"
)

// cloudInput is an input file for a context whose provider names
// example-idp, served from the URL it is read from.
const cloudInput = `contexts:
  - name: example-dev
    login:
      kind: oauth-browser
      issuer: https://idp.example.test
      issuerId: example-idp
      clientId: wso2-cli
`

func TestApplyCarriesTheIssuerIDIntoTheContext(t *testing.T) {
	shell, _, _ := newContextShell(t)
	mustRun(t, shell, "context", "apply", "-f", writeFile(t, cloudInput))

	login := contextNamed(t, loadDocument(t, shell), "example-dev").Login
	if login.IssuerID != "example-idp" || login.Issuer != "https://idp.example.test" {
		t.Fatalf("login = %+v, want the issuer and its declared identifier", login)
	}
}

// TestTheDocumentedIssuerIDExampleApplies applies the input file the context
// file reference shows for a provider that names another issuer.
func TestTheDocumentedIssuerIDExampleApplies(t *testing.T) {
	data, err := os.ReadFile("../../docs/reference/context-file.md")
	if err != nil {
		t.Fatal(err)
	}
	section := strings.SplitN(string(data), "### A provider that names another issuer\n", 2)
	if len(section) != 2 {
		t.Fatal("the reference has no section for a provider that names another issuer")
	}
	block := strings.SplitN(section[1], "```yaml\n", 2)
	if len(block) != 2 {
		t.Fatal("the section shows no YAML block")
	}
	example := strings.SplitN(block[1], "```", 2)[0]

	shell, _, _ := newContextShell(t)
	mustRun(t, shell, "context", "apply", "-f", writeFile(t, example))
	if login := contextNamed(t, loadDocument(t, shell), "example-dev").Login; login.IssuerID != "example-idp" {
		t.Fatalf("the documented example applied as %+v", login)
	}
}

func TestApplyRefusesAnIssuerIDTheContextCannotCarry(t *testing.T) {
	for name, content := range map[string]string{
		"an empty one": strings.Replace(cloudInput, "issuerId: example-idp", `issuerId: ""`, 1),
		"the issuer itself": strings.Replace(cloudInput, "issuerId: example-idp",
			"issuerId: https://idp.example.test", 1),
		"one on a product grant": cloudInput + `    products:
      apim:
        url: https://apim.example.test
        audience: apim
        grant:
          kind: federated
          issuer: https://apim.example.test/oauth2/token
          clientId: apim-cli
          issuerId: apim-idp
`,
	} {
		t.Run(name, func(t *testing.T) {
			shell, _, _ := newContextShell(t)
			code, _, errOut := run(t, shell, "context", "apply", "-f", writeFile(t, content), "--no-install")
			if code != exit.Usage {
				t.Fatalf("exit %d, want a usage refusal: %s", code, errOut)
			}
			if !strings.Contains(errOut, "issuer identifier") ||
				strings.Contains(errOut, "unknown field") {
				t.Errorf("the refusal does not name the issuer identifier:\n%s", errOut)
			}
			if document := loadDocument(t, shell); len(document.Contexts) != 0 {
				t.Errorf("a refused apply wrote contexts: %+v", document.Contexts)
			}
		})
	}
}

func TestContextCreateRecordsTheIssuerID(t *testing.T) {
	shell, _, _ := newContextShell(t)
	mustRun(t, shell, "context", "create", "cloud", "--issuer", "https://idp.example.test",
		"--issuer-id", "example-idp", "--client-id", "wso2-cli")

	login := contextNamed(t, loadDocument(t, shell), "cloud").Login
	if login.IssuerID != "example-idp" || login.Issuer != "https://idp.example.test" {
		t.Fatalf("login = %+v, want the issuer and its declared identifier", login)
	}
}

func TestContextCreateRefusesAnIssuerIDItCannotRecord(t *testing.T) {
	for name, testCase := range map[string]struct {
		args []string
		code string
	}{
		"without --issuer": {[]string{"--login-product", "iam", "--url", thunderURL, "--issuer-id", "example-idp"},
			"shell.conflicting_arguments"},
		"an empty one": {[]string{"--issuer", "https://idp.example.test", "--client-id", "c",
			"--issuer-id", ""}, "shell.invalid_argument"},
		"the issuer itself": {[]string{"--issuer", "https://idp.example.test", "--client-id", "c",
			"--issuer-id", "https://idp.example.test"}, "contexts.document_malformed"},
	} {
		t.Run(name, func(t *testing.T) {
			shell, _, _ := newContextShell(t)
			code, _, errOut := run(t, shell, append([]string{"context", "create", "cloud"}, testCase.args...)...)
			if code != exit.Usage {
				t.Fatalf("exit %d, want a usage refusal: %s", code, errOut)
			}
			if !strings.Contains(errOut, testCase.code) {
				t.Errorf("stderr does not carry %s:\n%s", testCase.code, errOut)
			}
			if _, err := os.Stat(contexts.Path(shell.StateRoot)); err == nil {
				t.Error("a refused create wrote the document")
			}
		})
	}
}
