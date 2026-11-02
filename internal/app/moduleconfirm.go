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

// declinedConfirmation is the refusal for a person who answered no. Nothing
// has been changed: a module asks before it acts, as the shell's own
// commands do.
func declinedConfirmation() problem.Problem {
	return problem.New(problem.CategoryUsage, "shell.cancelled",
		"the command was not confirmed").
		WithRecovery("Nothing was changed. Run the command again to start over.")
}

// moduleConfirmationCancelled turns a question the person cancelled into the
// shell's refusal for it. It takes the prompter's error as returned, so
// wizard.ErrAborted is still visible to errors.Is here; cancelled's wording
// promises nothing was written, which is true of the shell's own prompts; a
// module's deletion has not happened either, but "changed" is the word that
// is true of both.
func moduleConfirmationCancelled(err error) error {
	if errors.Is(err, wizard.ErrAborted) {
		return problem.New(problem.CategoryUsage, "shell.cancelled", "cancelled at a prompt").
			WithRecovery("Nothing was changed. Run the command again to start over.")
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
// is rendered (output.Named). An argument with a space is quoted, so a line
// that is copied back runs as it was.
func productCommandLine(namespace string, command, arguments []string) string {
	words := make([]string, 0, 2+len(command)+len(arguments))
	words = append(words, "wso2", namespace)
	words = append(words, command...)
	for _, argument := range arguments {
		if strings.ContainsAny(argument, " \t\"") {
			argument = `"` + strings.ReplaceAll(argument, `"`, `\"`) + `"`
		}
		words = append(words, argument)
	}
	return strings.Join(words, " ")
}
