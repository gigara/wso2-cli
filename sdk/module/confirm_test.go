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

package module_test

import (
	"context"
	"errors"
	"testing"

	"github.com/wso2/wso2-cli/sdk/module"
	"github.com/wso2/wso2-cli/sdk/result"
	"github.com/wso2/wso2-cli/sdk/testkit"
)

// confirmingCommand asks the shell to confirm a deletion, then reports whether
// it went ahead. deleted records whether the handler reached the act.
func confirmingCommand(question string, deleted *bool) module.Command {
	return module.Command{
		Path: []string{"delete"},
		Run: func(ctx context.Context, request module.Request) (result.Result, error) {
			if err := request.Prompt.Confirm(ctx, module.ConfirmRequest{Question: question}); err != nil {
				return result.Result{}, err
			}
			*deleted = true
			return result.New("probe.removal/v1").With("deleted", "Deleted", "thing"), nil
		},
	}
}

func TestAHandlerActsOnlyOnceTheShellConfirmed(t *testing.T) {
	var deleted bool
	outcome := testkit.Run(t.Context(), probeOptions(),
		[]module.Command{confirmingCommand("Delete the thing? This cannot be undone.", &deleted)},
		testkit.Invocation{Command: []string{"delete"}, Confirm: &testkit.Confirmation{}})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("a confirmed command failed: err=%v problem=%v", outcome.Err, outcome.Problem)
	}
	if !deleted {
		t.Fatal("the handler was confirmed and did not act")
	}
	if len(outcome.Confirmations) != 1 || outcome.Confirmations[0].Question != "Delete the thing? This cannot be undone." {
		t.Fatalf("the shell was asked %+v, want the handler's question once", outcome.Confirmations)
	}
}

func TestAHandlerReturnsTheShellsRefusalUnchanged(t *testing.T) {
	// The refusal is shell policy, so what the handler returns is exactly
	// what it was sent: the shell recognizes its own refusal and reports it.
	refusal := testkit.NonInteractive("wso2 probe delete")
	var deleted bool
	outcome := testkit.Run(t.Context(), probeOptions(),
		[]module.Command{confirmingCommand("Delete the thing?", &deleted)},
		testkit.Invocation{Command: []string{"delete"}, Confirm: &testkit.Confirmation{Refuse: refusal}})

	if outcome.Err != nil {
		t.Fatalf("Run failed: %v", outcome.Err)
	}
	if deleted {
		t.Fatal("the handler acted on a refused confirmation")
	}
	if outcome.Problem == nil || *outcome.Problem != *refusal {
		t.Fatalf("the handler returned %+v, want the shell's refusal %+v", outcome.Problem, refusal)
	}
}

func TestAConfirmationIsNotRequestedOnceTheContextEnded(t *testing.T) {
	// A cancelled handler must not put a question to a person who has
	// already stopped the command.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var err error
	command := module.Command{
		Path: []string{"delete"},
		Run: func(_ context.Context, request module.Request) (result.Result, error) {
			err = request.Prompt.Confirm(ctx, module.ConfirmRequest{Question: "Delete?"})
			return result.Result{}, err
		},
	}
	outcome := testkit.Run(t.Context(), probeOptions(), []module.Command{command},
		testkit.Invocation{Command: []string{"delete"}, Confirm: &testkit.Confirmation{}})

	if outcome.Err != nil {
		t.Fatalf("Run failed: %v", outcome.Err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Confirm returned %v, want the context's cancellation", err)
	}
	if len(outcome.Confirmations) != 0 {
		t.Fatalf("a cancelled handler still asked %+v", outcome.Confirmations)
	}
	if outcome.Problem == nil || outcome.Problem.Code != "probe.handler_failed" {
		t.Fatalf("the cancellation was reported as %+v, want a handler failure", outcome.Problem)
	}
}
