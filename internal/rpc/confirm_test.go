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

package rpc

import (
	"bytes"
	"testing"

	"github.com/wso2/wso2-cli/sdk/problem"
	"github.com/wso2/wso2-cli/sdk/protocol"
	"github.com/wso2/wso2-cli/sdk/protocol/contractv1"
)

// recordingConfirmer answers with a fixed outcome and records what it was
// asked.
type recordingConfirmer struct {
	refusal error
	asked   []string
}

func (c *recordingConfirmer) Confirm(question string) error {
	c.asked = append(c.asked, question)
	return c.refusal
}

// confirmRequest is the message a module sends to ask for confirmation.
func confirmRequest() *contractv1.Envelope {
	return &contractv1.Envelope{
		InvocationId:  testInvocationID,
		CorrelationId: "confirm-1",
		Message: &contractv1.Envelope_Confirm{Confirm: &contractv1.Confirm{
			Question: "Delete the API abc and every deployment of it?",
		}},
	}
}

// confirmingHello is the handshake of a module speaking the version that
// carries confirmation.
func confirmingHello() *contractv1.Envelope {
	hello := conformingHello()
	hello.GetHello().ProtocolVersions = []uint32{protocol.ConfirmationVersion}
	return hello
}

// runConfirmed drives a session speaking the confirmation version against a
// fixed module stream.
func runConfirmed(t *testing.T, confirmer Confirmer, fromModule *bytes.Buffer) (Outcome, []*contractv1.Envelope, error) {
	t.Helper()
	var toModule bytes.Buffer
	session := testSession()
	session.Resolved.ProtocolVersion = protocol.ConfirmationVersion
	session.Confirmer = confirmer
	outcome, err := session.Run(&toModule, fromModule, statusInvocation())
	return outcome, shellMessages(t, &toModule), err
}

// lastConfirmationAnswer returns the shell's answer to the module's
// confirmation request.
func lastConfirmationAnswer(t *testing.T, written []*contractv1.Envelope) *contractv1.Envelope {
	t.Helper()
	for _, envelope := range written {
		if envelope.GetConfirmed() != nil || envelope.GetConfirmationRefused() != nil {
			return envelope
		}
	}
	t.Fatal("the shell never answered the module's confirmation request")
	return nil
}

func TestAConfirmationRequestIsAnsweredWithTheConfirmersYes(t *testing.T) {
	confirmer := &recordingConfirmer{}

	outcome, written, err := runConfirmed(t, confirmer, moduleStream(t, confirmingHello(), confirmRequest(), statusResult()))

	if err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if outcome.Result.Schema != "reference.status/v1" {
		t.Errorf("the module's result did not survive the exchange: %+v", outcome.Result)
	}
	if len(confirmer.asked) != 1 || confirmer.asked[0] != "Delete the API abc and every deployment of it?" {
		t.Fatalf("the confirmer was asked %q, want the module's question once", confirmer.asked)
	}
	answer := lastConfirmationAnswer(t, written)
	if answer.GetConfirmed() == nil {
		t.Fatalf("the shell answered with %v, want a confirmation", answer.GetMessage())
	}
	if answer.GetInvocationId() != testInvocationID || answer.GetCorrelationId() != "confirm-1" {
		t.Errorf("the answer carries %q/%q, want the invocation's and the request's",
			answer.GetInvocationId(), answer.GetCorrelationId())
	}
}

func TestARefusedConfirmationIsAnsweredWithTheShellsProblemAndReportedInItsWords(t *testing.T) {
	// The module returns the refusal it was sent, and the shell reports its
	// own version of a refusal it issued, so a module cannot reword it.
	refusal := problem.New(problem.CategoryUsage, "shell.non_interactive",
		"wso2 reference status needs to be confirmed, and --no-input asked that nothing prompt").
		WithRecovery("Pass --yes to proceed without a prompt, or drop --no-input to be asked.")
	reworded := refusal
	reworded.Recovery = "Something the module wrote."
	confirmer := &recordingConfirmer{refusal: refusal}

	outcome, written, err := runConfirmed(t, confirmer,
		moduleStream(t, confirmingHello(), confirmRequest(), returnedProblem(reworded)))

	if err != nil {
		t.Fatalf("Run returned %v", err)
	}
	answer := lastConfirmationAnswer(t, written).GetConfirmationRefused()
	if answer == nil {
		t.Fatal("the shell did not refuse the confirmation")
	}
	if answer.GetProblem().GetCode() != refusal.Code || answer.GetProblem().GetRecovery() != refusal.Recovery {
		t.Errorf("the module was sent %+v, want the confirmer's refusal", answer.GetProblem())
	}
	if outcome.Problem == nil || *outcome.Problem != refusal {
		t.Errorf("the user is told %+v, want the shell's own refusal %+v", outcome.Problem, refusal)
	}
}

func TestAShellWithNoConfirmerRefusesRatherThanConfirming(t *testing.T) {
	_, written, err := runConfirmed(t, nil, moduleStream(t, confirmingHello(), confirmRequest(), statusResult()))

	if err != nil {
		t.Fatalf("Run returned %v", err)
	}
	refused := lastConfirmationAnswer(t, written).GetConfirmationRefused()
	if refused == nil {
		t.Fatal("a shell that can ask nobody confirmed, or left the request unanswered")
	}
	if refused.GetProblem().GetCategory() != string(problem.CategoryUsage) {
		t.Errorf("the refusal is in the %q class, want %q", refused.GetProblem().GetCategory(), problem.CategoryUsage)
	}
}

func TestABlankQuestionIsRefusedWithoutAskingAnyone(t *testing.T) {
	confirmer := &recordingConfirmer{}
	blank := confirmRequest()
	blank.GetConfirm().Question = "  "

	_, written, err := runConfirmed(t, confirmer, moduleStream(t, confirmingHello(), blank, statusResult()))

	if err != nil {
		t.Fatalf("Run returned %v", err)
	}
	refused := lastConfirmationAnswer(t, written).GetConfirmationRefused()
	if refused == nil {
		t.Fatal("a blank question was confirmed, or left unanswered")
	}
	if refused.GetProblem().GetCode() != "rpc.blank_question" {
		t.Errorf("the refusal is %q, want rpc.blank_question", refused.GetProblem().GetCode())
	}
	if len(confirmer.asked) != 0 {
		t.Errorf("a blank question was still put to the person: %q", confirmer.asked)
	}
}

func TestAnUntypedConfirmerFailureStillRefuses(t *testing.T) {
	confirmer := &recordingConfirmer{refusal: errUnexpected{}}

	_, written, err := runConfirmed(t, confirmer, moduleStream(t, confirmingHello(), confirmRequest(), statusResult()))

	if err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if lastConfirmationAnswer(t, written).GetConfirmationRefused() == nil {
		t.Fatal("an unexpected confirmer failure did not refuse")
	}
}

func TestAnUncorrelatedConfirmationRequestIsRefused(t *testing.T) {
	uncorrelated := confirmRequest()
	uncorrelated.CorrelationId = ""
	confirmer := &recordingConfirmer{}

	_, _, err := runConfirmed(t, confirmer, moduleStream(t, confirmingHello(), uncorrelated, statusResult()))

	if err == nil {
		t.Fatal("the shell answered a confirmation request that named no exchange")
	}
	if len(confirmer.asked) != 0 {
		t.Errorf("the confirmer was consulted %d times for an uncorrelated request", len(confirmer.asked))
	}
}

func TestAConfirmationRequestUnderAnOlderProtocolIsRefused(t *testing.T) {
	// A module that negotiated v1 and asks anyway is speaking a contract it
	// did not declare, and nobody is asked on its behalf.
	confirmer := &recordingConfirmer{}
	var toModule bytes.Buffer
	session := testSession()
	session.Confirmer = confirmer

	_, err := session.Run(&toModule, moduleStream(t, conformingHello(), confirmRequest(), statusResult()), statusInvocation())

	if got := problemCode(t, err); got != "rpc.unexpected_message" {
		t.Fatalf("the session failed with %q, want rpc.unexpected_message", got)
	}
	if len(confirmer.asked) != 0 {
		t.Errorf("the confirmer was consulted %d times under a protocol without confirmation", len(confirmer.asked))
	}
}

func TestAModuleThatKeepsAskingForConfirmationIsStopped(t *testing.T) {
	envelopes := []*contractv1.Envelope{confirmingHello()}
	for range MaxConfirmations + 1 {
		envelopes = append(envelopes, confirmRequest())
	}
	envelopes = append(envelopes, statusResult())

	_, _, err := runConfirmed(t, &recordingConfirmer{}, moduleStream(t, envelopes...))

	if got := problemCode(t, err); got != "rpc.too_many_confirmations" {
		t.Fatalf("the session failed with %q, want rpc.too_many_confirmations", got)
	}
}
