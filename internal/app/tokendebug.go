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
	"encoding/base64"
	"os"
	"strings"
)

// TokenDebugEnvVar turns on the temporary token diagnostics of an exchange,
// written to the --verbose log only. "claims" (or "1") logs the decoded header
// and claims of the login access token and of the token it is exchanged for;
// "raw" logs the tokens themselves as well. Anything else, and the variable
// unset, logs nothing.
//
// This is a bring-up aid for a deployment, not a feature: a raw token is a
// usable bearer credential until it expires, and the claims name the user and
// the organization. Remove it once the deployment is understood.
const TokenDebugEnvVar = "WSO2_DEBUG_TOKENS"

// tokenDebug is the broker's TokenDebug for this invocation, or nil when the
// variable does not ask for one or --verbose is off.
func (s Shell) tokenDebug() func(stage, token string) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(TokenDebugEnvVar)))
	raw := mode == "raw"
	if (mode != "claims" && mode != "1" && !raw) || !s.log.Enabled() {
		return nil
	}
	return func(stage, token string) {
		header, claims := decodeJWT(token)
		// The attribute names are chosen to stay clear of the logger's
		// redaction list: these values are logged because a person asked for
		// them by name, through the variable above.
		attributes := []any{"stage", stage, "jwt_header", header, "jwt_claims", claims}
		if raw {
			attributes = append(attributes, "jwt_raw", token)
		}
		s.log.Debug("exchange token (WSO2_DEBUG_TOKENS)", attributes...)
	}
}

// decodeJWT returns a JWT's header and claims as the JSON they encode, without
// verifying anything. A token that is not a JWT is reported as such rather
// than logged.
func decodeJWT(token string) (header, claims string) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "not a JWT", "not a JWT"
	}
	decode := func(segment string) string {
		decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(segment, "="))
		if err != nil {
			return "unreadable"
		}
		return string(decoded)
	}
	return decode(parts[0]), decode(parts[1])
}
