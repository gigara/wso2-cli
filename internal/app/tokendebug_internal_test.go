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

package app

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/wso2/wso2-cli/internal/output"
)

// sampleJWT is an unsigned token whose claims a test can recognise.
func sampleJWT() string {
	encode := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	return encode(`{"alg":"RS256","kid":"default-key"}`) + "." +
		encode(`{"iss":"platform-idp","aud":"https://api.example/ipaas","ouHandle":"acme"}`) + ".c2lnbmF0dXJl"
}

func verboseShell() (Shell, *bytes.Buffer) {
	var log bytes.Buffer
	logger := output.NewLogger()
	logger.Enable(&log, output.ModeTable)
	return Shell{log: logger}, &log
}

func TestTokenDebugIsOffUnlessAskedForAndVerbose(t *testing.T) {
	t.Setenv(TokenDebugEnvVar, "")
	shell, _ := verboseShell()
	if shell.tokenDebug() != nil {
		t.Error("token debugging is on with the variable unset")
	}

	t.Setenv(TokenDebugEnvVar, "claims")
	if (Shell{log: output.NewLogger()}).tokenDebug() != nil {
		t.Error("token debugging is on without --verbose")
	}

	t.Setenv(TokenDebugEnvVar, "yes please")
	if shell.tokenDebug() != nil {
		t.Error("token debugging is on for a value it does not name")
	}
}

func TestTokenDebugLogsClaimsButNotTheTokenUnlessRaw(t *testing.T) {
	token := sampleJWT()

	t.Setenv(TokenDebugEnvVar, "claims")
	shell, log := verboseShell()
	shell.tokenDebug()("exchanged", token)
	for _, want := range []string{"stage=exchanged", "platform-idp", "ouHandle", "default-key"} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("the log is missing %q:\n%s", want, log)
		}
	}
	if strings.Contains(log.String(), token) || strings.Contains(log.String(), "c2lnbmF0dXJl") {
		t.Errorf("claims mode logged the token itself:\n%s", log)
	}

	t.Setenv(TokenDebugEnvVar, "raw")
	shell, log = verboseShell()
	shell.tokenDebug()("login", token)
	if !strings.Contains(log.String(), token) {
		t.Errorf("raw mode did not log the token:\n%s", log)
	}
}

func TestDecodeJWTReportsATokenThatIsNotOne(t *testing.T) {
	header, claims := decodeJWT("opaque-token")
	if header != "not a JWT" || claims != "not a JWT" {
		t.Errorf("decodeJWT(opaque) = %q, %q", header, claims)
	}
}
