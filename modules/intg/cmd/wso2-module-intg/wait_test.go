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
	"time"

	"github.com/wso2/wso2-cli/sdk/problem"
	"github.com/wso2/wso2-cli/sdk/testkit"
)

// fastPolling makes every wait in the test poll at once.
func fastPolling(t *testing.T) {
	t.Helper()
	previous := pollInterval
	pollInterval = time.Millisecond
	t.Cleanup(func() { pollInterval = previous })
}

// The commands that wait declare a deadline the shell honours; the rest keep
// its default.
func TestTheWaitingCommandsDeclareALongerDeadline(t *testing.T) {
	declared := commands().Declare()
	for _, command := range declared.Commands {
		path := strings.Join(command.Path, " ")
		want := 0
		if path == "build run" || path == "component deploy" || path == "component promote" {
			want = int(waitDeadline / time.Second)
		}
		if command.TimeoutSeconds != want {
			t.Errorf("%q declares %d seconds, want %d", path, command.TimeoutSeconds, want)
		}
	}
}

// --- build run --wait -----------------------------------------------------

// buildWaitRoutes is order-api before and after a build is started: the
// listing gains b3, which runs and then ends as final.
func buildWaitRoutes(final string) map[string]stubRoute {
	return map[string]stubRoute{
		"GET /components/order-api/builds": {status: http.StatusOK, body: `{"items":[{"name":"b2","status":"Succeeded"}]}`,
			then: []stubRoute{{status: http.StatusOK,
				body: `{"items":[{"name":"b3","status":"Running","trigger":"manual"},{"name":"b2","status":"Succeeded"}]}`}}},
		"POST /components/order-api/builds": {status: http.StatusCreated, body: `{"message":"build triggered","success":true}`},
		"/components/order-api/builds/b3": {status: http.StatusOK, body: `{"name":"b3","status":"Running"}`,
			then: []stubRoute{{status: http.StatusOK, body: `{"name":"b3","status":"` + final +
				`","image":"registry.example/order-api:b3","commit":"` + fullCommit + `"}`}}},
	}
}

func TestBuildRunWaitFollowsTheNewBuildUntilItSucceeds(t *testing.T) {
	fastPolling(t)
	stub := newPlatformStub(t, buildWaitRoutes("Succeeded"))

	outcome := run([]string{"build", "run"}, []string{"order-api", "--wait"}, stub.endpoint(), &testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("build run --wait failed: %v %v", outcome.Err, outcome.Problem)
	}
	if outcome.Result.Schema != BuildSchema || field(outcome, "name") != "b3" || field(outcome, "status") != "Succeeded" {
		t.Errorf("result = %+v, want the new build b3, succeeded", outcome.Result.Fields)
	}
	if next := field(outcome, NextField); !strings.Contains(next, "wso2 intg component deploy order-api --env <environment> --build b3") {
		t.Errorf("next = %q, want it to name deploying b3", next)
	}
	if stub.paths[0] != "GET "+basePath+"/components/order-api/builds" || stub.paths[1] != "POST "+basePath+"/components/order-api/builds" {
		t.Errorf("calls = %v, want the listing read before the build is started", stub.paths)
	}
}

func TestBuildRunWaitReportsAFailedBuildAsAProblem(t *testing.T) {
	fastPolling(t)
	stub := newPlatformStub(t, buildWaitRoutes("Failed"))

	outcome := run([]string{"build", "run"}, []string{"order-api", "--wait"}, stub.endpoint(), &testkit.Access{Token: "t"})

	assertProblem(t, outcome, "intg.build_failed", problem.CategoryProductService)
	if !strings.Contains(outcome.Problem.Recovery, "wso2 intg build logs order-api b3") {
		t.Errorf("recovery = %q, want it to name the build's logs", outcome.Problem.Recovery)
	}
}

func TestBuildRunWaitStopsAtItsTimeout(t *testing.T) {
	fastPolling(t)
	routes := buildWaitRoutes("Succeeded")
	routes["/components/order-api/builds/b3"] = stubRoute{status: http.StatusOK, body: `{"name":"b3","status":"Running"}`}
	stub := newPlatformStub(t, routes)

	outcome := run([]string{"build", "run"}, []string{"order-api", "--wait", "--wait-timeout", "50ms"}, stub.endpoint(),
		&testkit.Access{Token: "t"})

	assertProblem(t, outcome, "intg.wait_timeout", problem.CategoryProductService)
	if !strings.Contains(outcome.Problem.Recovery, "wso2 intg build show order-api b3") {
		t.Errorf("recovery = %q, want it to name the build to follow", outcome.Problem.Recovery)
	}
}

func TestAWaitTimeoutBeyondTheDeclaredDeadlineIsRefused(t *testing.T) {
	stub := newPlatformStub(t, nil)

	outcome := run([]string{"build", "run"}, []string{"order-api", "--wait", "--wait-timeout", "1h"}, stub.endpoint(),
		&testkit.Access{Token: "t"})

	assertProblem(t, outcome, "intg.invalid_argument", problem.CategoryUsage)
	if stub.calls != 0 {
		t.Errorf("a refused command line reached the API (%v)", stub.paths)
	}
}

// --- component deploy and promote --wait ----------------------------------

// rollout is development answering the old release, then the new one in
// progress, then the new one ending as final.
func rollout(environment, release, final string) map[string]stubRoute {
	return map[string]stubRoute{
		"GET /components/order-api/deployments?environmentId=" + environment: {status: http.StatusOK, body: activeDeployment,
			then: []stubRoute{
				{status: http.StatusOK, body: `{"releaseId":"` + release + `","deploymentStatusV2":"IN_PROGRESS"}`},
				{status: http.StatusOK, body: `{"releaseId":"` + release + `","deploymentStatusV2":"` + final + `"}`},
			}},
	}
}

func TestComponentDeployWaitFollowsTheNewReleaseUntilItIsActive(t *testing.T) {
	fastPolling(t)
	routes := rollout("development", "order-api-r4", "ACTIVE")
	routes["POST /components/order-api/deployments"] = stubRoute{status: http.StatusOK, body: `{"message":"order-api-r4"}`}
	stub := newPlatformStub(t, deploymentRoutes(routes))

	outcome := run([]string{"component", "deploy"}, []string{"order-api", "--env", "development", "--project", "orders",
		"--wait"}, stub.endpoint(), &testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("component deploy --wait failed: %v %v", outcome.Err, outcome.Problem)
	}
	if field(outcome, "release") != "order-api-r4" || field(outcome, "status") != "ACTIVE" {
		t.Errorf("result = %+v, want order-api-r4 active", outcome.Result.Fields)
	}
}

func TestComponentDeployWithoutWaitReportsTheRequest(t *testing.T) {
	stub := newPlatformStub(t, deploymentRoutes(map[string]stubRoute{
		"POST /components/order-api/deployments": {status: http.StatusOK, body: `{"message":"order-api-r4"}`},
	}))

	outcome := run([]string{"component", "deploy"}, []string{"order-api", "--env", "development", "--project", "orders"},
		stub.endpoint(), &testkit.Access{Token: "t"})

	if field(outcome, "status") != "requested" {
		t.Errorf("status = %q, want requested", field(outcome, "status"))
	}
	assertNoReadAfterWrite(t, stub)
}

func TestComponentDeployWaitReportsAFailedRollout(t *testing.T) {
	fastPolling(t)
	routes := rollout("development", "order-api-r4", "ERROR")
	routes["POST /components/order-api/deployments"] = stubRoute{status: http.StatusOK, body: `{"message":"order-api-r4"}`}
	stub := newPlatformStub(t, deploymentRoutes(routes))

	outcome := run([]string{"component", "deploy"}, []string{"order-api", "--env", "development", "--project", "orders",
		"--wait"}, stub.endpoint(), &testkit.Access{Token: "t"})

	assertProblem(t, outcome, "intg.deployment_failed", problem.CategoryProductService)
	if !strings.Contains(outcome.Problem.Recovery, "wso2 intg component logs order-api --env development") {
		t.Errorf("recovery = %q, want it to name the component's logs", outcome.Problem.Recovery)
	}
}

func TestComponentPromoteWaitFollowsTheTargetEnvironment(t *testing.T) {
	fastPolling(t)
	routes := map[string]stubRoute{
		"GET /components/order-api/deployments?environmentId=production": {status: http.StatusOK, body: ``,
			then: []stubRoute{{status: http.StatusOK, body: `{"releaseId":"order-api-r3","deploymentStatusV2":"ACTIVE"}`}}},
		"POST /components/order-api/deployments/promote": {status: http.StatusOK, body: `{"message":"order-api-r3"}`},
	}
	stub := newPlatformStub(t, deploymentRoutes(routes))

	outcome := run([]string{"component", "promote"}, []string{"order-api", "--from", "development", "--to", "production",
		"--project", "orders", "--wait"}, stub.endpoint(), &testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("component promote --wait failed: %v %v", outcome.Err, outcome.Problem)
	}
	if field(outcome, "status") != "ACTIVE" {
		t.Errorf("status = %q, want ACTIVE", field(outcome, "status"))
	}
}

// assertNoReadAfterWrite checks a command without --wait made no call after
// its write: it reports the request and leaves.
func assertNoReadAfterWrite(t *testing.T, stub *platformStub) {
	t.Helper()
	last := stub.paths[len(stub.paths)-1]
	if strings.HasPrefix(last, "GET ") {
		t.Errorf("the command read %s after its write, without --wait", last)
	}
}
