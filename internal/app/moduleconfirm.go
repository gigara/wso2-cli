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
	"errors"
	"strings"

	"github.com/wso2/wso2-cli/internal/rpc"
	"github.com/wso2/wso2-cli/internal/wizard"
	"github.com/wso2/wso2-cli/sdk/problem"
)

// moduleConfirmer is how a product module's confirmation request is answered:
// the one place the shell decides, for a module, whether anything may be
// asked, draws the module's question, and writes every refusal.
//
// It is the shell's own rule for its own destructive commands applied to a
// module's, down to the refusal: mayPrompt decides from --no-input,
// WSO2_NO_INPUT and the terminal, in that order; the prompter draws the
// question as a form or a line prompt; and where nothing may be asked the
// refusal is the same shell.non_interactive that ws product remove returns,
// naming the control that fired and --yes, the flag the command guideline
// gives every destructive command. The module contributes the question and
// nothing else; the subject of the refusal is the command the shell itself
// parsed. See docs/adr/0021-a-shell-drawn-confirmation-in-the-module-contract.md.
func (s Shell) moduleConfirmer(namespace string, command, arguments []string, noInput bool) rpc.Confirmer {
	commandLine := productCommandLine(namespace, command, arguments)
	return rpc.ConfirmerFunc(func(question string) error {
		if may, reason := s.mayPrompt(noInput); !may {
			return nonInteractiveConfirmation(commandLine, reason)
		}
		// The prompter is asked directly, not through s.confirm: that
		// helper already turns wizard.ErrAborted into the shell's own
		// refusal, whose wording moduleConfirmationCancelled replaces.
		confirmed, err := s.prompter().Confirm(question, false)
		if err != nil {
			return moduleConfirmationCancelled(err)
		}
		if !confirmed {
			return declinedConfirmation()
		}
		return nil
	})
}

// declinedConfirmation is the refusal for a person who answered no. The
// wording promises only that the module did not go on: an invocation may ask
// more than once, and an earlier step may already have happened.
func declinedConfirmation() problem.Problem {
	return problem.New(problem.CategoryUsage, "shell.cancelled",
		"the command was not confirmed").
		WithRecovery("Nothing further was done. Run the command again to start over.")
}

// moduleConfirmationCancelled turns a question the person cancelled into the
// shell's refusal for it. It takes the prompter's error as returned, so
// wizard.ErrAborted is still visible to errors.Is here. cancelled's wording
// promises nothing was written, which is true of the shell's own prompts but
// not of a module that asks after an earlier step; this one promises only
// that nothing further was done.
func moduleConfirmationCancelled(err error) error {
	if errors.Is(err, wizard.ErrAborted) {
		return problem.New(problem.CategoryUsage, "shell.cancelled", "cancelled at a prompt").
			WithRecovery("Nothing further was done. Run the command again to start over.")
	}
	return err
}

// productCommandLine renders the product command the shell parsed, as a
// person would type it again: the shell's name, the namespace, the command
// path, and the module's own arguments. It is the subject of the
// non-interactive refusal, which a module cannot name for itself. The shell's
// flags (--context, --output, --no-input) are not repeated: the refusal is
// about --yes, which is the module's.
//
// The name is written as "wso2" like every other suggestion and renamed as it
// is rendered (output.Named). An argument the shell would not read literally
// is single-quoted, so a line that is copied back runs as it was: a space
// would split it, and a $ or a backtick would expand.
func productCommandLine(namespace string, command, arguments []string) string {
	words := make([]string, 0, 2+len(command)+len(arguments))
	words = append(words, "wso2", namespace)
	words = append(words, command...)
	for _, argument := range arguments {
		words = append(words, shellWord(argument))
	}
	return strings.Join(words, " ")
}

// shellWord renders one argument so that a POSIX shell reads it as the same
// string: as it is when every character is one the shell takes literally,
// and single-quoted otherwise, with a single quote inside written as '\”.
func shellWord(argument string) string {
	if argument != "" && strings.IndexFunc(argument, func(r rune) bool { return !literalInAShell(r) }) < 0 {
		return argument
	}
	return "'" + strings.ReplaceAll(argument, "'", `'\''`) + "'"
}

// literalInAShell reports whether a POSIX shell reads r as itself, unquoted.
func literalInAShell(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
		strings.ContainsRune("_-./:@%+=,", r)
}
