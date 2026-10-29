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

package main

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wso2/wso2-cli/sdk/module"
	"github.com/wso2/wso2-cli/sdk/result"
)

// ComponentLogsSchema identifies a component's runtime log in one environment.
const ComponentLogsSchema = "intg.componentLogs/v1"

// defaultLogTail is how many of the newest lines component logs shows when
// --tail is not given.
const defaultLogTail = 100

// componentLogsFlags are the options of "wso2 intg component logs".
type componentLogsFlags struct {
	env     string
	since   string
	tail    int
	project string
}

// componentLogs answers "wso2 intg component logs <component> --env <environment>".
//
// The log is queried from the observability proxy the way the console queries
// it, scoped to the component in one environment of its project. The observer
// takes the project, component and environment by name and resolves them
// itself.
func componentLogs(command *cobra.Command, flags *componentLogsFlags) module.Handler {
	return func(ctx context.Context, request module.Request) (result.Result, error) {
		recovery := "Run wso2 intg component logs <component> --env <environment> [--since <duration>] [--tail <n>]."
		componentID, err := exactlyOneArgument(command.Flags().Args(), "name the component whose logs to show",
			recovery)
		if err != nil {
			return result.Result{}, err
		}
		if err := requireFlag(flags.env, "--env", recovery); err != nil {
			return result.Result{}, err
		}
		since, err := time.ParseDuration(flags.since)
		if err != nil || since <= 0 {
			return result.Result{}, usageProblem("--since "+flags.since+" is not a length of time",
				"Give --since as a duration such as 15m, 1h or 24h.")
		}
		if flags.tail < 0 {
			return result.Result{}, usageProblem("--tail "+strconv.Itoa(flags.tail)+" is not a number of lines",
				"Give --tail a positive number, or 0 for every line in the window.")
		}
		if err := requireObservability(request); err != nil {
			return result.Result{}, err
		}
		client, err := clientFor(ctx, request)
		if err != nil {
			return result.Result{}, err
		}
		endpoint := request.Context.Endpoint
		environments, err := listEnvironments(ctx, client, endpoint)
		if err != nil {
			return result.Result{}, err
		}
		env, err := resolveEnvironment(environments, flags.env, "--env")
		if err != nil {
			return result.Result{}, err
		}
		project, err := componentProject(ctx, client, componentID, flags.project, endpoint)
		if err != nil {
			return result.Result{}, err
		}
		end := time.Now().UTC().Truncate(time.Second)
		lines, total, err := queryLogs(ctx, request, client.Token,
			map[string]string{"project": project, "component": componentID, "environment": env.ID},
			end.Add(-since), end, flags.tail, "read the logs of "+componentID+" in "+env.ID)
		if err != nil {
			return result.Result{}, err
		}
		next := "Run wso2 intg component logs " + componentID + " --env " + env.ID + " --since <duration> to look " +
			"further back."
		if len(lines) == 0 {
			next = "There is no output in that time. Run wso2 intg deployment show " + componentID + " --env " +
				env.ID + " to see whether it runs there."
		}
		return result.New(ComponentLogsSchema).
			With("component", "Component", componentID).
			With("environment", "Environment", env.ID).
			With("since", "Since", since.String()).
			With("lines", "Lines", lineCount(len(lines), total)).
			With("log", "Log", orDash(strings.Join(lines, "\n"))).
			With(NextField, "Next", next), nil
	}
}
