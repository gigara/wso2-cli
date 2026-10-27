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
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wso2/wso2-cli/sdk/module"
	"github.com/wso2/wso2-cli/sdk/problem"
	"github.com/wso2/wso2-cli/sdk/result"
)

// BuildStartSchema identifies an accepted build request.
const BuildStartSchema = "intg.buildStart/v1"

// BuildsSchema identifies a component's build listing.
const BuildsSchema = "intg.builds/v1"

// BuildSchema identifies one build.
const BuildSchema = "intg.build/v1"

// BuildLogsSchema identifies one build's log.
const BuildLogsSchema = "intg.buildLogs/v1"

// fullCommitSHA is the only commit form a build can be pinned to: the build
// fetches the commit as a ref, which git allows only for a full SHA.
var fullCommitSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// shortCommitLength is how much of a commit a listing shows.
const shortCommitLength = 12

// build is the part of the API's WorkflowRun this module reports. Builds are
// named by the platform, and a name is what build show and build logs take.
type build struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Trigger   string `json:"trigger"`
	Component string `json:"componentName"`
	Project   string `json:"projectName"`
	Commit    string `json:"commit"`
	Image     string `json:"image"`
	Started   string `json:"startedAt"`
	Completed string `json:"completedAt"`
	Tasks     []struct {
		Name  string `json:"name"`
		Phase string `json:"phase"`
	} `json:"tasks"`
}

func (b build) shortCommit() string {
	if len(b.Commit) > shortCommitLength {
		return b.Commit[:shortCommitLength]
	}
	return orDash(b.Commit)
}

// steps is one "name: phase" line per step, or "-" when the build states none.
func (b build) steps() string {
	lines := make([]string, len(b.Tasks))
	for i, task := range b.Tasks {
		lines[i] = task.Name + ": " + orDash(task.Phase)
	}
	return orDash(strings.Join(lines, "\n"))
}

func buildsPath(componentID string) string {
	return componentPath(componentID) + "/builds"
}

func buildPath(componentID, buildName string) string {
	return buildsPath(componentID) + "/" + url.PathEscape(buildName)
}

// buildListRecovery is where to find a build's name.
func buildListRecovery(componentID string) string {
	return "Run wso2 intg build list " + componentID + " to see the names of its builds."
}

// buildRunFlags are the options of "wso2 intg build run".
type buildRunFlags struct {
	commit string
}

// buildRun answers "wso2 intg build run <component> [--commit <sha>]".
//
// The API accepts the request without naming the build it started, so the
// result names the listing where it appears, newest first.
func buildRun(command *cobra.Command, flags *buildRunFlags) module.Handler {
	return func(ctx context.Context, request module.Request) (result.Result, error) {
		componentID, err := exactlyOneArgument(command.Flags().Args(), "name the component to build",
			"Run wso2 intg build run <component> [--commit <sha>].")
		if err != nil {
			return result.Result{}, err
		}
		body := map[string]string{}
		commit := "branch head"
		if flags.commit != "" {
			if !fullCommitSHA.MatchString(flags.commit) {
				return result.Result{}, usageProblem("--commit "+flags.commit+" is not a full 40-character commit SHA, "+
					"and a build can fetch only a full one", "Give the full SHA, or leave --commit out to build the "+
					"head of the component's branch.")
			}
			body["sha"] = strings.ToLower(flags.commit)
			commit = body["sha"]
		}
		client, err := clientFor(ctx, request)
		if err != nil {
			return result.Result{}, err
		}
		var accepted struct {
			Message string `json:"message"`
		}
		if err := client.Post(ctx, buildsPath(componentID), body, &accepted); err != nil {
			if refused, ok := refusal(err, http.StatusBadGateway); ok {
				return result.Result{}, problem.New(problem.CategoryProductService, "intg.call_failed",
					"the Integration Platform could not start a build of "+componentID+": "+refusalWords(refused)).
					WithRecovery("Run wso2 intg build run " + componentID + " again.")
			}
			return result.Result{}, callFailedNaming(err, "start a build of "+componentID, request.Context.Endpoint,
				componentListRecovery(""))
		}
		return result.New(BuildStartSchema).
			With("component", "Component", componentID).
			With("commit", "Commit", commit).
			With("message", "Message", orDash(accepted.Message)).
			With(NextField, "Next", "Run wso2 intg build list "+componentID+" to see it; the newest build is "+
				"listed first."), nil
	}
}

// buildList answers "wso2 intg build list <component>".
//
// The API returns at most 20 builds, newest first, with no continuation
// cursor.
func buildList(command *cobra.Command) module.Handler {
	return func(ctx context.Context, request module.Request) (result.Result, error) {
		componentID, err := exactlyOneArgument(command.Flags().Args(), "name the component whose builds to list",
			"Run wso2 intg build list <component>.")
		if err != nil {
			return result.Result{}, err
		}
		client, err := clientFor(ctx, request)
		if err != nil {
			return result.Result{}, err
		}
		var listing struct {
			Items []build `json:"items"`
		}
		if err := client.Get(ctx, buildsPath(componentID), &listing); err != nil {
			return result.Result{}, callFailedNaming(err, "list the builds of "+componentID, request.Context.Endpoint,
				componentListRecovery(""))
		}
		report := result.New(BuildsSchema).
			With("component", "Component", componentID).
			With("count", "Builds", strconv.Itoa(len(listing.Items))).
			WithColumn("name", "Name").
			WithColumn("status", "Status").
			WithColumn("trigger", "Trigger").
			WithColumn("commit", "Commit").
			WithColumn("started", "Started").
			WithColumn("completed", "Completed")
		for _, b := range listing.Items {
			report = report.WithRow(b.Name, orDash(b.Status), orDash(b.Trigger), b.shortCommit(), orDash(b.Started),
				orDash(b.Completed))
		}
		next := "Run wso2 intg build show " + componentID + " <build> to see one build."
		if len(listing.Items) == 0 {
			next = "It has no builds yet. Start one with wso2 intg build run " + componentID + "."
		}
		return report.With(NextField, "Next", next), nil
	}
}

// buildShow answers "wso2 intg build show <component> <build>".
func buildShow(command *cobra.Command) module.Handler {
	return func(ctx context.Context, request module.Request) (result.Result, error) {
		componentID, buildName, err := exactlyTwoArguments(command.Flags().Args(),
			"name the component and the build to show", "Run wso2 intg build show <component> <build>.")
		if err != nil {
			return result.Result{}, err
		}
		client, err := clientFor(ctx, request)
		if err != nil {
			return result.Result{}, err
		}
		var found build
		if err := client.Get(ctx, buildPath(componentID, buildName), &found); err != nil {
			return result.Result{}, buildReadFailed(err, "show the build "+buildName+" of "+componentID,
				request.Context.Endpoint, componentID)
		}
		if found.Name == "" {
			found.Name = buildName
		}
		if found.Component == "" {
			found.Component = componentID
		}
		next := "Run wso2 intg build logs " + componentID + " " + found.Name + " to see its output."
		switch found.Status {
		case "Failed":
			next = "Run wso2 intg build logs " + componentID + " " + found.Name + " to see why it failed."
		case "Pending", "Running":
			next = "Run wso2 intg build show " + componentID + " " + found.Name + " again to follow it."
		}
		return result.New(BuildSchema).
			With("name", "Name", found.Name).
			With("component", "Component", found.Component).
			With("project", "Project", orDash(found.Project)).
			With("status", "Status", orDash(found.Status)).
			With("trigger", "Trigger", orDash(found.Trigger)).
			With("commit", "Commit", orDash(found.Commit)).
			With("image", "Image", orDash(found.Image)).
			With("started", "Started", orDash(found.Started)).
			With("completed", "Completed", orDash(found.Completed)).
			With("steps", "Steps", found.steps()).
			With(NextField, "Next", next), nil
	}
}

// buildReadFailed maps a refused read of one build. The API answers a build
// name it does not know with a 500 rather than a 404, so a 500 here points at
// the listing too.
func buildReadFailed(err error, attempted, endpoint, componentID string) error {
	if refused, ok := refusal(err, http.StatusInternalServerError); ok {
		return problem.New(problem.CategoryProductService, "intg.call_failed",
			"the Integration Platform could not "+attempted+": "+refusalWords(refused)).
			WithRecovery("Check the build name: " + buildListRecovery(componentID) + " If the name is right, retry " +
				"in a moment.")
	}
	return callFailedNaming(err, attempted, endpoint, buildListRecovery(componentID))
}

// buildLogsFlags are the options of "wso2 intg build logs".
type buildLogsFlags struct {
	tail int
}

// logsQueryPath is the observability proxy's log query, reached through the
// integration API's /observability pass-through with the caller's token. It
// is the request the Integration Platform console makes for a build's log.
//
// The API's own build-log route is not used: it asks the proxy for
// /api/logs/build/{build}, a path the proxy does not serve (it routes only
// /wso2cloud-obs/), so it fails with a 500 for every caller.
const logsQueryPath = "/observability/wso2cloud-obs/api/v1/logs/query"

// logsQueryLimit is the most lines one build log is read with, the same bound
// the API's own route uses.
const logsQueryLimit = 1000

// logWindowMargin widens a build's run on both sides, so lines stamped just
// before it started or after it completed are not cut off.
const logWindowMargin = 5 * time.Minute

// logWindow is the time range to search for a build's log: its run, widened
// by logWindowMargin, up to now while it has not completed, and the last 30
// days when it states no start.
func (b build) logWindow(now time.Time) (time.Time, time.Time) {
	start, err := time.Parse(time.RFC3339, b.Started)
	if err != nil {
		return now.Add(-30 * 24 * time.Hour), now
	}
	end := now
	if completed, err := time.Parse(time.RFC3339, b.Completed); err == nil {
		end = completed.Add(logWindowMargin)
	}
	return start.Add(-logWindowMargin), end
}

// buildLogs answers "wso2 intg build logs <component> <build> [--tail <n>]".
//
// The build is read first, which confirms it exists and bounds the search to
// its run. The log is then queried as the workflow run of that name. The
// proxy sets searchScope.namespace from the token's organization, so the
// member is sent empty.
func buildLogs(command *cobra.Command, flags *buildLogsFlags) module.Handler {
	return func(ctx context.Context, request module.Request) (result.Result, error) {
		componentID, buildName, err := exactlyTwoArguments(command.Flags().Args(),
			"name the component and the build whose log to show", "Run wso2 intg build logs <component> <build>.")
		if err != nil {
			return result.Result{}, err
		}
		if flags.tail < 0 {
			return result.Result{}, usageProblem("--tail "+strconv.Itoa(flags.tail)+" is not a number of lines",
				"Give --tail a positive number, or leave it out to see the whole log.")
		}
		client, err := clientFor(ctx, request)
		if err != nil {
			return result.Result{}, err
		}
		var found build
		if err := client.Get(ctx, buildPath(componentID, buildName), &found); err != nil {
			return result.Result{}, buildReadFailed(err, "find the build "+buildName+" of "+componentID,
				request.Context.Endpoint, componentID)
		}
		start, end := found.logWindow(time.Now().UTC())
		// The newest lines are asked for first when only a tail is wanted, so
		// a long log is not cut off at the limit before its end is reached.
		order, limit := "asc", logsQueryLimit
		if flags.tail > 0 && flags.tail < logsQueryLimit {
			order, limit = "desc", flags.tail
		}
		query := map[string]any{
			"searchScope": map[string]string{"namespace": "", "workflowRunName": buildName},
			"startTime":   start.Format(time.RFC3339),
			"endTime":     end.Format(time.RFC3339),
			"limit":       limit,
			"sortOrder":   order,
		}
		var answer struct {
			Logs []struct {
				Timestamp string `json:"timestamp"`
				Log       string `json:"log"`
			} `json:"logs"`
			Total int `json:"total"`
		}
		if err := client.Post(ctx, logsQueryPath, query, &answer); err != nil {
			return result.Result{}, callFailed(err, "read the log of the build "+buildName+" of "+componentID,
				request.Context.Endpoint)
		}
		lines := make([]string, len(answer.Logs))
		for i, entry := range answer.Logs {
			line := strings.TrimRight(entry.Log, "\n")
			if entry.Timestamp != "" {
				line = entry.Timestamp + " " + line
			}
			lines[i] = line
		}
		if order == "desc" {
			slices.Reverse(lines)
		}
		count := strconv.Itoa(len(lines))
		if answer.Total > len(lines) {
			count += " of " + strconv.Itoa(answer.Total)
		}
		next := "Run wso2 intg build show " + componentID + " " + buildName + " to see its status."
		if len(lines) == 0 {
			next = "There is no output yet; it appears once the build runs. Run wso2 intg build show " +
				componentID + " " + buildName + " to see its status."
		}
		return result.New(BuildLogsSchema).
			With("build", "Build", buildName).
			With("component", "Component", componentID).
			With("lines", "Lines", count).
			With("log", "Log", orDash(strings.Join(lines, "\n"))).
			With(NextField, "Next", next), nil
	}
}
