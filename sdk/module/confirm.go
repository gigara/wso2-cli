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

package module

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/wso2/wso2-cli/sdk/protocol"
	"github.com/wso2/wso2-cli/sdk/protocol/contractv1"
)

// ConfirmRequest is the question a handler asks the shell to put to the
// person running the command before it acts on something that cannot be
// undone.
//
// The module writes the question because it alone knows what it is about to
// delete. Everything else about asking belongs to the shell: whether a prompt
// is allowed at all, how it is drawn, and what a refusal says. See
// docs/adr/0021-a-shell-drawn-confirmation-in-the-module-contract.md.
type ConfirmRequest struct {
	// Question is the complete question, such as "Delete the API abc and
	// every deployment of it? This cannot be undone." The shell draws it
	// verbatim with its own yes/no hint, so it ends with a question mark and
	// carries no hint of its own.
	Question string
}

// Confirmer is the shell's prompt as a handler sees it.
//
// It is an interface so a module author can test a handler against a stub,
// and so the SDK can replace the implementation without changing handlers.
type Confirmer interface {
	// Confirm asks the shell to put the question to the person. It returns
	// nil when they answered yes, and a problem.Problem otherwise: the
	// shell's shell.cancelled when they answered no or cancelled, or its
	// shell.non_interactive when nothing could be asked because --no-input,
	// WSO2_NO_INPUT or a standard input that is not a terminal forbade it,
	// whose recovery names --yes. A handler returns that problem unchanged:
	// the refusal is shell policy, and the shell renders it and chooses the
	// exit class.
	//
	// A handler whose command line carried --yes does not call this. The
	// flag is the module's own, and skipping the question is what it means.
	Confirm(ctx context.Context, request ConfirmRequest) error
}

// Confirm performs one confirmation exchange over the invocation's streams.
//
// It shares the broker's mutex and correlation counter, so a confirmation and
// an access request can never interleave on the one stream. The wait for the
// shell's answer carries no timer of its own for the reasons Acquire gives:
// the shell closes this module's input on every path it can end an invocation
// by, and that closure ends the wait.
func (b *streamBroker) Confirm(ctx context.Context, request ConfirmRequest) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("module: confirmation was not requested: %w", err)
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.requests++
	correlationID := "confirm-" + strconv.Itoa(b.requests)
	if err := b.writer.WriteEnvelope(&contractv1.Envelope{
		InvocationId:  b.invocationID,
		CorrelationId: correlationID,
		Message: &contractv1.Envelope_Confirm{Confirm: &contractv1.Confirm{
			Question: request.Question,
		}},
	}); err != nil {
		return fmt.Errorf("module: cannot ask the shell for confirmation: %w", err)
	}

	envelope, err := b.reader.ReadEnvelope()
	if err != nil {
		return fmt.Errorf("module: cannot read the shell's answer to a confirmation request: %w", err)
	}
	// As for access, the answer is proved to be this invocation's answer to
	// this question before it is believed, so a replayed yes cannot confirm
	// a different deletion.
	if envelope.GetInvocationId() != b.invocationID {
		return errors.New("module: the shell answered a confirmation request for another invocation")
	}
	if envelope.GetCorrelationId() != correlationID {
		return errors.New("module: the shell answered a confirmation request that was not made")
	}

	switch {
	case envelope.GetConfirmed() != nil:
		return nil
	case envelope.GetConfirmationRefused() != nil:
		refusal, decodeErr := protocol.DecodeProblem(envelope.GetConfirmationRefused().GetProblem())
		if decodeErr != nil {
			return fmt.Errorf("module: the shell refused confirmation unreportably: %w", decodeErr)
		}
		return refusal
	default:
		return fmt.Errorf("module: the shell answered a confirmation request with %s",
			protocol.DescribeMessage(envelope))
	}
}
