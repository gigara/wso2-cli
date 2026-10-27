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
	"net/http"
	"strings"
	"testing"

	"github.com/wso2/wso2-cli/sdk/problem"
	"github.com/wso2/wso2-cli/sdk/testkit"
)

const fullCommit = "0123456789abcdef0123456789abcdef01234567"

const orderAPIBuilds = `{"items":[
  {"name":"order-api-workflow-b2","status":"Running","trigger":"manual","commit":"` + fullCommit + `",
   "startedAt":"2026-10-27T10:05:00Z"},
  {"name":"order-api-workflow-b1","status":"Failed","trigger":"initial","commit":"fedcba9876543210fedcba9876543210fedcba98",
   "startedAt":"2026-10-27T09:00:00Z","completedAt":"2026-10-27T09:04:00Z"}]}`

const failedBuild = `{"name":"order-api-workflow-b1","status":"Failed","trigger":"initial",
  "componentName":"order-api","projectName":"orders","commit":"fedcba9876543210fedcba9876543210fedcba98",
  "image":"","startedAt":"2026-10-27T09:00:00Z","completedAt":"2026-10-27T09:04:00Z",
  "tasks":[{"name":"clone","phase":"Succeeded"},{"name":"build","phase":"Failed"}]}`

// --- build run ------------------------------------------------------------

func TestBuildRunBuildsTheBranchHeadWhenNoCommitIsGiven(t *testing.T) {
	stub := newPlatformStub(t, map[string]stubRoute{
		"POST /components/order-api/builds": {http.StatusCreated, `{"message":"build triggered","success":true}`},
	})

	outcome := run([]string{"build", "run"}, []string{"order-api"}, stub.endpoint(), &testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("build run failed: %v %v", outcome.Err, outcome.Problem)
	}
	if stub.method != http.MethodPost || stub.path != basePath+"/components/order-api/builds" {
		t.Errorf("the API was called with %s %s", stub.method, stub.path)
	}
	if sent := decodeBody(t, stub.body); len(sent) != 0 {
		t.Errorf("sent %v, want an empty body, which builds the branch head", sent)
	}
	if outcome.Result.Schema != BuildStartSchema || field(outcome, "component") != "order-api" ||
		field(outcome, "commit") != "branch head" {
		t.Errorf("result = %+v", outcome.Result.Fields)
	}
	if next := field(outcome, NextField); !strings.Contains(next, "wso2 intg build list order-api") {
		t.Errorf("next = %q, want it to name wso2 intg build list order-api", next)
	}
}

func TestBuildRunPinsAFullCommit(t *testing.T) {
	stub := newPlatformStub(t, map[string]stubRoute{
		"POST /components/order-api/builds": {http.StatusCreated, `{"message":"build triggered","success":true}`},
	})

	outcome := run([]string{"build", "run"}, []string{"order-api", "--commit", strings.ToUpper(fullCommit)},
		stub.endpoint(), &testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("build run failed: %v %v", outcome.Err, outcome.Problem)
	}
	if sent := decodeBody(t, stub.body); sent["sha"] != fullCommit {
		t.Errorf("sent %v, want sha %s in lower case", sent, fullCommit)
	}
	if field(outcome, "commit") != fullCommit {
		t.Errorf("commit = %q", field(outcome, "commit"))
	}
}

// The build fetches the commit as a ref, which git allows only for a full
// SHA, so a short one is refused before a build is started that would fail.
func TestBuildRunRefusesAShortCommit(t *testing.T) {
	stub := newPlatformStub(t, nil)

	outcome := run([]string{"build", "run"}, []string{"order-api", "--commit", "0123456"}, stub.endpoint(),
		&testkit.Access{Token: "t"})

	assertProblem(t, outcome, "intg.invalid_argument", problem.CategoryUsage)
	if stub.calls != 0 || len(outcome.AccessRequests) != 0 {
		t.Errorf("a refused command line reached the API (%v)", stub.paths)
	}
}

func TestBuildRunMapsItsRefusals(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		code     string
		recovery string
	}{
		{"unknown component", http.StatusNotFound, `{"error":"Not Found","message":"component not found"}`,
			"intg.not_found", "wso2 intg component list"},
		{"git integration down", http.StatusBadGateway,
			`{"error":"Bad Gateway","message":"git integration unavailable; retry the build"}`,
			"intg.call_failed", "wso2 intg build run order-api"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := newPlatformStub(t, map[string]stubRoute{"POST /components/order-api/builds": {c.status, c.body}})

			outcome := run([]string{"build", "run"}, []string{"order-api"}, stub.endpoint(), &testkit.Access{Token: "t"})

			assertProblem(t, outcome, c.code, problem.CategoryProductService)
			if !strings.Contains(outcome.Problem.Recovery, c.recovery) {
				t.Errorf("recovery = %q, want it to name %q", outcome.Problem.Recovery, c.recovery)
			}
		})
	}
}

// --- build list -----------------------------------------------------------

func TestBuildListReportsTheBuildsNewestFirst(t *testing.T) {
	stub := newPlatformStub(t, map[string]stubRoute{"/components/order-api/builds": {http.StatusOK, orderAPIBuilds}})

	outcome := run([]string{"build", "list"}, []string{"order-api"}, stub.endpoint(), &testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("build list failed: %v %v", outcome.Err, outcome.Problem)
	}
	if outcome.Result.Schema != BuildsSchema || field(outcome, "count") != "2" ||
		field(outcome, "component") != "order-api" {
		t.Errorf("result = %+v", outcome.Result.Fields)
	}
	assertColumns(t, outcome, "name", "status", "trigger", "commit", "started", "completed")
	assertRow(t, outcome, 0, "order-api-workflow-b2|Running|manual|0123456789ab|2026-10-27T10:05:00Z|-")
	assertRow(t, outcome, 1, "order-api-workflow-b1|Failed|initial|fedcba987654|2026-10-27T09:00:00Z|2026-10-27T09:04:00Z")
	assertEndsWithNext(t, outcome)
}

func TestAnEmptyBuildListNamesBuildRun(t *testing.T) {
	stub := newPlatformStub(t, map[string]stubRoute{"/components/order-api/builds": {http.StatusOK, `{"items":[]}`}})

	outcome := run([]string{"build", "list"}, []string{"order-api"}, stub.endpoint(), &testkit.Access{Token: "t"})

	if next := field(outcome, NextField); !strings.Contains(next, "wso2 intg build run order-api") {
		t.Errorf("next = %q, want it to name wso2 intg build run order-api", next)
	}
}

// --- build show -----------------------------------------------------------

func TestBuildShowReportsAFailedBuildAndPointsAtItsLogs(t *testing.T) {
	stub := newPlatformStub(t, map[string]stubRoute{
		"/components/order-api/builds/order-api-workflow-b1": {http.StatusOK, failedBuild},
	})

	outcome := run([]string{"build", "show"}, []string{"order-api", "order-api-workflow-b1"}, stub.endpoint(),
		&testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("build show failed: %v %v", outcome.Err, outcome.Problem)
	}
	if outcome.Result.Schema != BuildSchema {
		t.Errorf("schema = %q, want %q", outcome.Result.Schema, BuildSchema)
	}
	want := []struct{ name, value string }{
		{"name", "order-api-workflow-b1"}, {"component", "order-api"}, {"project", "orders"}, {"status", "Failed"},
		{"trigger", "initial"}, {"commit", "fedcba9876543210fedcba9876543210fedcba98"}, {"image", "-"},
		{"started", "2026-10-27T09:00:00Z"}, {"completed", "2026-10-27T09:04:00Z"},
		{"steps", "clone: Succeeded\nbuild: Failed"},
	}
	for i, w := range want {
		got := outcome.Result.Fields[i]
		if got.Name != w.name || got.Value != w.value {
			t.Errorf("field %d = %s=%q, want %s=%q", i, got.Name, got.Value, w.name, w.value)
		}
	}
	if next := field(outcome, NextField); !strings.Contains(next, "wso2 intg build logs order-api order-api-workflow-b1") {
		t.Errorf("next = %q, want it to name the logs of this build", next)
	}
}

func TestBuildShowOfARunningBuildSaysToFollowIt(t *testing.T) {
	stub := newPlatformStub(t, map[string]stubRoute{
		"/components/order-api/builds/b2": {http.StatusOK, `{"name":"b2","status":"Running","trigger":"manual"}`},
	})

	outcome := run([]string{"build", "show"}, []string{"order-api", "b2"}, stub.endpoint(), &testkit.Access{Token: "t"})

	if next := field(outcome, NextField); !strings.Contains(next, "wso2 intg build show order-api b2") {
		t.Errorf("next = %q, want it to name build show again", next)
	}
}

// The API answers an unknown build with a 500 rather than a 404, so that
// refusal also points at the listing.
func TestBuildShowOfAnUnknownBuildNamesTheListing(t *testing.T) {
	stub := newPlatformStub(t, map[string]stubRoute{
		"/components/order-api/builds/nope": {http.StatusInternalServerError,
			`{"error":"Internal Server Error","message":"failed to get build status"}`},
	})

	outcome := run([]string{"build", "show"}, []string{"order-api", "nope"}, stub.endpoint(), &testkit.Access{Token: "t"})

	assertProblem(t, outcome, "intg.call_failed", problem.CategoryProductService)
	if !strings.Contains(outcome.Problem.Recovery, "wso2 intg build list order-api") {
		t.Errorf("recovery = %q, want it to name wso2 intg build list order-api", outcome.Problem.Recovery)
	}
}

func TestBuildShowNeedsAComponentAndABuild(t *testing.T) {
	for _, arguments := range [][]string{nil, {"order-api"}, {"a", "b", "c"}} {
		outcome := run([]string{"build", "show"}, arguments, "https://unused.example", &testkit.Access{Token: "t"})

		assertProblem(t, outcome, "intg.invalid_argument", problem.CategoryUsage)
	}
}

// --- build logs -----------------------------------------------------------

const buildLog = `{"init":{"log":"","status":"","steps":[]},
  "build":{"log":"2026-10-27T09:00:01Z cloning\n2026-10-27T09:01:00Z compiling\n2026-10-27T09:04:00Z error: missing module\n","status":"","steps":[]},
  "deploy":{"log":"","status":"","steps":[]}}`

// logsStub answers the build b1 of order-api, in the project orders, and its
// log; and b2, which has produced no output yet.
func logsStub(t *testing.T, log string) *platformStub {
	return newPlatformStub(t, map[string]stubRoute{
		"/components/order-api/builds/b1":      {http.StatusOK, `{"name":"b1","projectName":"orders","status":"Failed"}`},
		"/components/order-api/builds/b1/logs": {http.StatusOK, log},
		"/components/order-api/builds/b2":      {http.StatusOK, `{"name":"b2","projectName":"orders","status":"Running"}`},
		"/components/order-api/builds/b2/logs": {http.StatusOK, log},
	})
}

// The log service filters by project, and the API passes projectName to it as
// given without resolving it from the component, so a request without one is
// refused with an opaque 500. The build itself names its project, so the log
// is asked for with that.
func TestBuildLogsAsksForTheLogWithTheBuildsProject(t *testing.T) {
	stub := logsStub(t, buildLog)

	outcome := run([]string{"build", "logs"}, []string{"order-api", "b1"}, stub.endpoint(), &testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("build logs failed: %v %v", outcome.Err, outcome.Problem)
	}
	want := []string{"GET " + basePath + "/components/order-api/builds/b1", "GET " + basePath + "/components/order-api/builds/b1/logs"}
	if strings.Join(stub.paths, ", ") != strings.Join(want, ", ") {
		t.Errorf("calls = %v, want %v", stub.paths, want)
	}
	if stub.query != "projectName=orders" {
		t.Errorf("the log was asked for with query %q, want projectName=orders", stub.query)
	}
}

func TestBuildLogsWithAProjectAsksForTheLogDirectly(t *testing.T) {
	stub := logsStub(t, buildLog)

	outcome := run([]string{"build", "logs"}, []string{"order-api", "b1", "--project", "orders"}, stub.endpoint(),
		&testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("build logs failed: %v %v", outcome.Err, outcome.Problem)
	}
	if len(stub.paths) != 1 || stub.query != "projectName=orders" {
		t.Errorf("calls = %v with query %q, want only the log, with projectName=orders", stub.paths, stub.query)
	}
}

func TestBuildLogsReportsTheWholeLog(t *testing.T) {
	stub := logsStub(t, buildLog)

	outcome := run([]string{"build", "logs"}, []string{"order-api", "b1"}, stub.endpoint(), &testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("build logs failed: %v %v", outcome.Err, outcome.Problem)
	}
	if outcome.Result.Schema != BuildLogsSchema || field(outcome, "lines") != "3" {
		t.Errorf("result = %+v", outcome.Result.Fields)
	}
	if log := field(outcome, "log"); !strings.HasPrefix(log, "2026-10-27T09:00:01Z cloning") ||
		!strings.HasSuffix(log, "error: missing module") {
		t.Errorf("log = %q, want every line without a trailing newline", log)
	}
	assertEndsWithNext(t, outcome)
}

func TestBuildLogsTailKeepsTheLastLines(t *testing.T) {
	stub := logsStub(t, buildLog)

	outcome := run([]string{"build", "logs"}, []string{"order-api", "b1", "--tail", "1"}, stub.endpoint(),
		&testkit.Access{Token: "t"})

	if log := field(outcome, "log"); log != "2026-10-27T09:04:00Z error: missing module" {
		t.Errorf("log = %q, want only the last line", log)
	}
	if field(outcome, "lines") != "1 of 3" {
		t.Errorf("lines = %q, want 1 of 3", field(outcome, "lines"))
	}
}

func TestBuildLogsOfABuildWithNoOutputYetSaysSo(t *testing.T) {
	stub := logsStub(t, `{"init":{"log":""},"build":{"log":""},"deploy":{"log":""}}`)

	outcome := run([]string{"build", "logs"}, []string{"order-api", "b2"}, stub.endpoint(), &testkit.Access{Token: "t"})

	if field(outcome, "lines") != "0" || field(outcome, "log") != "-" {
		t.Errorf("result = %+v, want no lines", outcome.Result.Fields)
	}
	if next := field(outcome, NextField); !strings.Contains(next, "wso2 intg build show order-api b2") {
		t.Errorf("next = %q, want it to name build show", next)
	}
}

func TestBuildLogsRefusesANegativeTail(t *testing.T) {
	outcome := run([]string{"build", "logs"}, []string{"order-api", "b1", "--tail", "-3"}, "https://unused.example",
		&testkit.Access{Token: "t"})

	assertProblem(t, outcome, "intg.invalid_argument", problem.CategoryUsage)
}
