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
	"errors"
	"strings"
	"testing"

	"github.com/wso2/wso2-cli/internal/output"
	"github.com/wso2/wso2-cli/sdk/problem"
)

const (
	confirmQuestion    = "Delete the API abc and every deployment of it? This cannot be undone."
	confirmCommandLine = "wso2 apim api delete abc"
)

// answering builds a shell whose prompts read from input, as a test that
// cannot hand over a real terminal does, and returns its standard error.
func answering(input string) (Shell, *bytes.Buffer) {
	errOut := &bytes.Buffer{}
	shell := Shell{Streams: output.Streams{Out: &bytes.Buffer{}, Err: errOut}}
	if input != "" {
		shell.Reader = strings.NewReader(input)
	}
	return shell, errOut
}

func refusalOf(t *testing.T, err error) problem.Problem {
	t.Helper()
	var typed problem.Problem
	if !errors.As(err, &typed) {
		t.Fatalf("the confirmer failed with an untyped error: %v", err)
	}
	return typed
}

func TestAModuleQuestionAnsweredYesConfirms(t *testing.T) {
	shell, errOut := answering("y\n")
	err := shell.moduleConfirmer("apim", []string{"api", "delete"}, []string{"abc"}, false).Confirm(confirmQuestion)
	if err != nil {
		t.Fatalf("a yes was reported as %v", err)
	}
	if !strings.Contains(errOut.String(), confirmQuestion) {
		t.Errorf("the module's question was not drawn:\n%s", errOut)
	}
}

func TestAModuleQuestionAnsweredNoIsCancelled(t *testing.T) {
	shell, _ := answering("n\n")
	err := shell.moduleConfirmer("apim", []string{"api", "delete"}, []string{"abc"}, false).Confirm(confirmQuestion)
	refusal := refusalOf(t, err)
	if refusal.Code != "shell.cancelled" || refusal.Category != problem.CategoryUsage {
		t.Fatalf("a no was refused as %s/%s, want usage/shell.cancelled", refusal.Category, refusal.Code)
	}
	if !strings.Contains(refusal.Recovery, "Nothing was changed") {
		t.Errorf("the refusal does not say nothing changed: %q", refusal.Recovery)
	}
}

func TestAModuleQuestionWithNoAnswerAtAllIsCancelled(t *testing.T) {
	// Input that ends before an answer is a no, the fail-closed reading
	// the shell's own confirmations give an empty line.
	shell, _ := answering("")
	shell.Reader = emptyReader{}
	err := shell.moduleConfirmer("apim", []string{"api", "delete"}, []string{"abc"}, false).Confirm(confirmQuestion)
	if refusalOf(t, err).Code != "shell.cancelled" {
		t.Fatalf("an empty stream was reported as %v, want shell.cancelled", err)
	}
}

// TestAModuleQuestionIsRefusedWhereNothingMayBeAsked covers the three
// controls in the order mayPrompt consults them, and that the refusal is the
// same shell.non_interactive ws product remove returns: the parsed command as
// its subject, --yes beside the one way out that applies. The negative
// assertions are the load-bearing half: a recovery listing every way out
// would satisfy the positive ones alone.
func TestAModuleQuestionIsRefusedWhereNothingMayBeAsked(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		noInput      bool
		env          string
		wantReason   string
		wantRecovery string
		wantAbsent   []string
	}{
		{
			name: "--no-input", noInput: true, env: "1",
			wantReason:   "--no-input asked that nothing prompt",
			wantRecovery: "Pass --yes to proceed without a prompt, or drop --no-input to be asked.",
			wantAbsent:   []string{NoInputEnvVar, "terminal"},
		},
		{
			name: "WSO2_NO_INPUT", env: "1",
			wantReason:   NoInputEnvVar + " asked that nothing prompt",
			wantRecovery: "Pass --yes to proceed without a prompt, or unset " + NoInputEnvVar + " to be asked.",
			wantAbsent:   []string{"--no-input to be asked", "terminal"},
		},
		{
			name:         "piped standard input",
			wantReason:   reasonNotATerminal,
			wantRecovery: "Pass --yes to proceed without a prompt, or run this where standard input is a terminal.",
			wantAbsent:   []string{"--no-input", NoInputEnvVar},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv(NoInputEnvVar, testCase.env)
			// Reader is left nil, so the prompt would read the process's
			// own standard input, which go test does not attach a terminal
			// to.
			shell, errOut := answering("")
			err := shell.moduleConfirmer("apim", []string{"api", "delete"}, []string{"abc"}, testCase.noInput).
				Confirm(confirmQuestion)
			refusal := refusalOf(t, err)
			if refusal.Code != "shell.non_interactive" || refusal.Category != problem.CategoryUsage {
				t.Fatalf("refused as %s/%s, want usage/shell.non_interactive", refusal.Category, refusal.Code)
			}
			wantMessage := confirmCommandLine + " needs to be confirmed, and " + testCase.wantReason
			if refusal.Message != wantMessage {
				t.Errorf("message is %q, want %q", refusal.Message, wantMessage)
			}
			if refusal.Recovery != testCase.wantRecovery {
				t.Errorf("recovery is %q, want %q", refusal.Recovery, testCase.wantRecovery)
			}
			for _, absent := range testCase.wantAbsent {
				if strings.Contains(refusal.Recovery, absent) {
					t.Errorf("the recovery names %q, a way out that does not apply: %q", absent, refusal.Recovery)
				}
			}
			if errOut.Len() != 0 {
				t.Errorf("a refused question was still drawn:\n%s", errOut)
			}
		})
	}
}

func TestTheProductCommandLineQuotesAnArgumentWithASpace(t *testing.T) {
	got := productCommandLine("apim", []string{"project", "delete"}, []string{"my project", "--force"})
	if want := `wso2 apim project delete "my project" --force`; got != want {
		t.Fatalf("productCommandLine = %q, want %q", got, want)
	}
}
