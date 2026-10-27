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

const twoEnvironments = `{"items":[
  {"id":"development","name":"Development","critical":false,"dpId":"dp-eu"},
  {"id":"production","name":"Production","critical":true,"dpId":"dp-eu"}]}`

const activeDeployment = `{"releaseId":"order-api-r3","deploymentStatusV2":"ACTIVE","cron":"",
  "build":{"buildId":"order-api-workflow-b2","deployedAt":"2026-10-27T11:00:00Z","commit":{"sha":"01234567"}}}`

// deploymentRoutes is a deployment of order-api, in the project orders, whose
// pipeline promotes development to production. order-api runs in development
// and is not deployed in production; its build b2 succeeded and b1 failed.
func deploymentRoutes(extra map[string]stubRoute) map[string]stubRoute {
	routes := map[string]stubRoute{
		"/environments":    {http.StatusOK, twoEnvironments},
		"/projects/orders": {http.StatusOK, `{"id":"orders","defaultDeploymentPipelineId":"default"}`},
		"/deploymentpipelines/default": {http.StatusOK,
			`{"name":"default","promotionPaths":[{"sourceEnvironment":"development","targetEnvironments":["production"]}]}`},
		"GET /components/order-api/deployments?environmentId=development": {http.StatusOK, activeDeployment},
		"GET /components/order-api/deployments?environmentId=production":  {http.StatusOK, ``},
		"/components/order-api/builds/b2": {http.StatusOK,
			`{"name":"b2","status":"Succeeded","image":"registry.example/order-api:b2"}`},
		"/components/order-api/builds/b1": {http.StatusOK, `{"name":"b1","status":"Failed","image":""}`},
	}
	for path, route := range extra {
		routes[path] = route
	}
	return routes
}

// --- environment list -----------------------------------------------------

func TestEnvironmentListReportsEveryEnvironment(t *testing.T) {
	stub := newPlatformStub(t, deploymentRoutes(nil))

	outcome := run([]string{"environment", "list"}, nil, stub.endpoint(), &testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("environment list failed: %v %v", outcome.Err, outcome.Problem)
	}
	if outcome.Result.Schema != EnvironmentsSchema || field(outcome, "count") != "2" {
		t.Errorf("result = %+v", outcome.Result.Fields)
	}
	assertColumns(t, outcome, "id", "name", "production", "dataPlane")
	assertRow(t, outcome, 0, "development|Development|no|dp-eu")
	assertRow(t, outcome, 1, "production|Production|yes|dp-eu")
	assertEndsWithNext(t, outcome)
}

// --- deployment list and show ---------------------------------------------

func TestDeploymentListReportsTheComponentInEveryEnvironment(t *testing.T) {
	stub := newPlatformStub(t, deploymentRoutes(nil))

	outcome := run([]string{"deployment", "list"}, []string{"order-api"}, stub.endpoint(), &testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("deployment list failed: %v %v", outcome.Err, outcome.Problem)
	}
	if outcome.Result.Schema != DeploymentsSchema || field(outcome, "component") != "order-api" {
		t.Errorf("result = %+v", outcome.Result.Fields)
	}
	assertColumns(t, outcome, "environment", "status", "release", "build", "commit", "deployed")
	assertRow(t, outcome, 0, "development|ACTIVE|order-api-r3|order-api-workflow-b2|01234567|2026-10-27T11:00:00Z")
	assertRow(t, outcome, 1, "production|not deployed|-|-|-|-")
	assertEndsWithNext(t, outcome)
}

func TestDeploymentShowTakesAnEnvironmentByItsDisplayName(t *testing.T) {
	stub := newPlatformStub(t, deploymentRoutes(nil))

	outcome := run([]string{"deployment", "show"}, []string{"order-api", "--env", "Development"}, stub.endpoint(),
		&testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("deployment show failed: %v %v", outcome.Err, outcome.Problem)
	}
	if stub.query != "environmentId=development" {
		t.Errorf("the deployment was read with %q, want the environment's ID", stub.query)
	}
	want := []struct{ name, value string }{
		{"component", "order-api"}, {"environment", "development"}, {"status", "ACTIVE"},
		{"release", "order-api-r3"}, {"build", "order-api-workflow-b2"}, {"commit", "01234567"},
		{"deployed", "2026-10-27T11:00:00Z"}, {"schedule", "-"},
	}
	for i, w := range want {
		got := outcome.Result.Fields[i]
		if got.Name != w.name || got.Value != w.value {
			t.Errorf("field %d = %s=%q, want %s=%q", i, got.Name, got.Value, w.name, w.value)
		}
	}
}

func TestDeploymentShowWhereNothingIsDeployedNamesHowToGetItThere(t *testing.T) {
	stub := newPlatformStub(t, deploymentRoutes(nil))

	outcome := run([]string{"deployment", "show"}, []string{"order-api", "--env", "production"}, stub.endpoint(),
		&testkit.Access{Token: "t"})

	if field(outcome, "status") != "not deployed" {
		t.Errorf("status = %q, want not deployed", field(outcome, "status"))
	}
	if next := field(outcome, NextField); !strings.Contains(next, "wso2 intg component promote order-api") {
		t.Errorf("next = %q, want it to name component promote", next)
	}
}

func TestAnUnknownEnvironmentIsRefusedNamingTheEnvironments(t *testing.T) {
	stub := newPlatformStub(t, deploymentRoutes(nil))

	outcome := run([]string{"deployment", "show"}, []string{"order-api", "--env", "staging"}, stub.endpoint(),
		&testkit.Access{Token: "t"})

	assertProblem(t, outcome, "intg.invalid_argument", problem.CategoryUsage)
	for _, id := range []string{"development", "production"} {
		if !strings.Contains(outcome.Problem.Recovery, id) {
			t.Errorf("recovery = %q, want it to name %s", outcome.Problem.Recovery, id)
		}
	}
	if stub.calls != 1 {
		t.Errorf("calls = %v, want only the environment listing", stub.paths)
	}
}

// --- component deploy -----------------------------------------------------

func TestComponentDeployReleasesTheCurrentImageWhereThePipelineStarts(t *testing.T) {
	stub := newPlatformStub(t, deploymentRoutes(map[string]stubRoute{
		"POST /components/order-api/deployments": {http.StatusOK, `{"message":"order-api-r4"}`},
	}))

	outcome := run([]string{"component", "deploy"}, []string{"order-api", "--env", "development", "--project", "orders"},
		stub.endpoint(), &testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("component deploy failed: %v %v", outcome.Err, outcome.Problem)
	}
	if stub.method != http.MethodPost || stub.path != basePath+"/components/order-api/deployments" ||
		stub.query != "projectName=orders" {
		t.Errorf("last call = %s %s?%s", stub.method, stub.path, stub.query)
	}
	sent := decodeBody(t, stub.body)
	if sent["environmentId"] != "development" || sent["imageId"] != "" || len(sent) != 2 {
		t.Errorf("sent %v, want the environment and an empty image, which releases the current one", sent)
	}
	if outcome.Result.Schema != DeploySchema || field(outcome, "release") != "order-api-r4" ||
		field(outcome, "build") != "current image" {
		t.Errorf("result = %+v", outcome.Result.Fields)
	}
	if next := field(outcome, NextField); !strings.Contains(next, "wso2 intg deployment show order-api --env development") {
		t.Errorf("next = %q, want it to name deployment show", next)
	}
}

func TestComponentDeployOfASucceededBuildSendsItsName(t *testing.T) {
	stub := newPlatformStub(t, deploymentRoutes(map[string]stubRoute{
		"POST /components/order-api/deployments": {http.StatusOK, `{"message":"order-api-r4"}`},
	}))

	outcome := run([]string{"component", "deploy"}, []string{"order-api", "--env", "development", "--project", "orders",
		"--build", "b2"}, stub.endpoint(), &testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("component deploy failed: %v %v", outcome.Err, outcome.Problem)
	}
	if sent := decodeBody(t, stub.body); sent["imageId"] != "b2" {
		t.Errorf("sent %v, want imageId b2", sent)
	}
}

// The API falls back to the current image when a build cannot be resolved and
// still reports success, so a build that cannot be deployed is refused first.
func TestComponentDeployRefusesABuildThatDidNotSucceed(t *testing.T) {
	stub := newPlatformStub(t, deploymentRoutes(nil))

	outcome := run([]string{"component", "deploy"}, []string{"order-api", "--env", "development", "--project", "orders",
		"--build", "b1"}, stub.endpoint(), &testkit.Access{Token: "t"})

	assertProblem(t, outcome, "intg.build_not_deployable", problem.CategoryUsage)
	assertNoWrite(t, stub)
}

// A first deploy goes where the pipeline starts; later environments are
// reached by promotion, which is what the pipeline exists to enforce.
func TestComponentDeployRefusesAnEnvironmentThePipelineDoesNotStartAt(t *testing.T) {
	stub := newPlatformStub(t, deploymentRoutes(nil))

	outcome := run([]string{"component", "deploy"}, []string{"order-api", "--env", "production", "--project", "orders"},
		stub.endpoint(), &testkit.Access{Token: "t"})

	assertProblem(t, outcome, "intg.not_pipeline_entry", problem.CategoryUsage)
	for _, want := range []string{"wso2 intg component deploy order-api --env development",
		"wso2 intg component promote order-api --from development --to production"} {
		if !strings.Contains(outcome.Problem.Recovery, want) {
			t.Errorf("recovery = %q, want it to name %q", outcome.Problem.Recovery, want)
		}
	}
	assertNoWrite(t, stub)
}

func TestComponentDeployFindsTheProjectWhenNoneIsGiven(t *testing.T) {
	stub := newPlatformStub(t, deploymentRoutes(map[string]stubRoute{
		"/projects":                              {http.StatusOK, `{"items":[{"id":"orders"}]}`},
		"/projects/orders/components":            {http.StatusOK, ordersComponents},
		"POST /components/order-api/deployments": {http.StatusOK, `{"message":"order-api-r4"}`},
	}))

	outcome := run([]string{"component", "deploy"}, []string{"order-api", "--env", "development"}, stub.endpoint(),
		&testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("component deploy failed: %v %v", outcome.Err, outcome.Problem)
	}
	if stub.query != "projectName=orders" {
		t.Errorf("the deploy was sent with %q, want the project the listing named", stub.query)
	}
}

// --- component promote ----------------------------------------------------

func TestComponentPromoteSendsTheSourceReleaseAndThePipeline(t *testing.T) {
	stub := newPlatformStub(t, deploymentRoutes(map[string]stubRoute{
		"POST /components/order-api/deployments/promote": {http.StatusOK, `{"message":"order-api-r3"}`},
	}))

	outcome := run([]string{"component", "promote"}, []string{"order-api", "--from", "development", "--to", "Production",
		"--project", "orders"}, stub.endpoint(), &testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("component promote failed: %v %v", outcome.Err, outcome.Problem)
	}
	if stub.path != basePath+"/components/order-api/deployments/promote" || stub.query != "projectName=orders" {
		t.Errorf("last call = %s %s?%s", stub.method, stub.path, stub.query)
	}
	sent := decodeBody(t, stub.body)
	want := map[string]any{"sourceReleaseId": "order-api-r3", "sourceEnvironmentId": "development",
		"targetEnvironmentId": "production", "deploymentPipelineId": "default"}
	for key, value := range want {
		if sent[key] != value {
			t.Errorf("sent %s = %v, want %v", key, sent[key], value)
		}
	}
	if outcome.Result.Schema != PromotionSchema || field(outcome, "release") != "order-api-r3" ||
		field(outcome, "to") != "production" {
		t.Errorf("result = %+v", outcome.Result.Fields)
	}
}

func TestComponentPromoteFromAnEnvironmentWithNothingDeployedIsRefused(t *testing.T) {
	stub := newPlatformStub(t, deploymentRoutes(nil))

	outcome := run([]string{"component", "promote"}, []string{"order-api", "--from", "production", "--to", "development",
		"--project", "orders"}, stub.endpoint(), &testkit.Access{Token: "t"})

	assertProblem(t, outcome, "intg.not_deployed", problem.CategoryProductService)
	assertNoWrite(t, stub)
}

func TestComponentPromoteRefusesTheSameEnvironmentTwice(t *testing.T) {
	stub := newPlatformStub(t, deploymentRoutes(nil))

	outcome := run([]string{"component", "promote"}, []string{"order-api", "--from", "development", "--to", "Development",
		"--project", "orders"}, stub.endpoint(), &testkit.Access{Token: "t"})

	assertProblem(t, outcome, "intg.invalid_argument", problem.CategoryUsage)
	assertNoWrite(t, stub)
}

// The API answers a hop the pipeline does not allow with a 403, which here is
// a pipeline rule rather than a missing permission.
func TestComponentPromoteReportsAHopThePipelineRefuses(t *testing.T) {
	stub := newPlatformStub(t, deploymentRoutes(map[string]stubRoute{
		"POST /components/order-api/deployments/promote": {http.StatusForbidden,
			`{"error":"Forbidden","message":"promotion from development to production is not allowed"}`},
	}))

	outcome := run([]string{"component", "promote"}, []string{"order-api", "--from", "development", "--to", "production",
		"--project", "orders"}, stub.endpoint(), &testkit.Access{Token: "t"})

	assertProblem(t, outcome, "intg.promotion_refused", problem.CategoryProductService)
	if !strings.Contains(outcome.Problem.Message, "is not allowed") {
		t.Errorf("message = %q, want the API's words", outcome.Problem.Message)
	}
}

// --- component stop and redeploy ------------------------------------------

func TestComponentStopStopsItInOneEnvironment(t *testing.T) {
	stub := newPlatformStub(t, deploymentRoutes(map[string]stubRoute{
		"PUT /components/order-api/deployments/stop": {http.StatusOK, `{"message":"stopped"}`},
	}))

	outcome := run([]string{"component", "stop"}, []string{"order-api", "--env", "development"}, stub.endpoint(),
		&testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("component stop failed: %v %v", outcome.Err, outcome.Problem)
	}
	if stub.method != http.MethodPut || stub.path != basePath+"/components/order-api/deployments/stop" {
		t.Errorf("last call = %s %s", stub.method, stub.path)
	}
	if sent := decodeBody(t, stub.body); sent["environment"] != "development" || len(sent) != 1 {
		t.Errorf("sent %v, want only the environment", sent)
	}
	if outcome.Result.Schema != StopSchema || field(outcome, "state") != "stopped" {
		t.Errorf("result = %+v", outcome.Result.Fields)
	}
	if next := field(outcome, NextField); !strings.Contains(next, "wso2 intg component redeploy order-api --env development") {
		t.Errorf("next = %q, want it to name redeploy", next)
	}
}

func TestComponentRedeployRestartsTheDeployedRelease(t *testing.T) {
	stub := newPlatformStub(t, deploymentRoutes(map[string]stubRoute{
		"POST /components/order-api/deployments/redeploy": {http.StatusOK, `{"message":"redeployed"}`},
	}))

	outcome := run([]string{"component", "redeploy"}, []string{"order-api", "--env", "development"}, stub.endpoint(),
		&testkit.Access{Token: "t"})

	if outcome.Err != nil || outcome.Problem != nil {
		t.Fatalf("component redeploy failed: %v %v", outcome.Err, outcome.Problem)
	}
	if stub.method != http.MethodPost || stub.query != "environment=development" {
		t.Errorf("last call = %s %s?%s", stub.method, stub.path, stub.query)
	}
	if outcome.Result.Schema != RedeploySchema || field(outcome, "environment") != "development" {
		t.Errorf("result = %+v", outcome.Result.Fields)
	}
}

func TestStopAndRedeployWhereNothingIsDeployedNameTheDeployments(t *testing.T) {
	for _, command := range []string{"stop", "redeploy"} {
		stub := newPlatformStub(t, deploymentRoutes(map[string]stubRoute{
			"PUT /components/order-api/deployments/stop":      {http.StatusNotFound, `{"error":"Not Found","message":"no deployment"}`},
			"POST /components/order-api/deployments/redeploy": {http.StatusNotFound, `{"error":"Not Found","message":"no deployment"}`},
		}))

		outcome := run([]string{"component", command}, []string{"order-api", "--env", "production"}, stub.endpoint(),
			&testkit.Access{Token: "t"})

		assertProblem(t, outcome, "intg.not_found", problem.CategoryProductService)
		if !strings.Contains(outcome.Problem.Recovery, "wso2 intg deployment list order-api") {
			t.Errorf("%s recovery = %q, want it to name deployment list", command, outcome.Problem.Recovery)
		}
	}
}

func TestDeploymentCommandsNeedAnEnvironment(t *testing.T) {
	for _, command := range [][]string{{"component", "deploy"}, {"component", "stop"}, {"component", "redeploy"},
		{"deployment", "show"}} {
		stub := newPlatformStub(t, nil)

		outcome := run(command, []string{"order-api"}, stub.endpoint(), &testkit.Access{Token: "t"})

		assertProblem(t, outcome, "intg.invalid_argument", problem.CategoryUsage)
		if stub.calls != 0 {
			t.Errorf("%v without --env reached the API (%v)", command, stub.paths)
		}
	}
}

// assertNoWrite checks a refused command changed nothing: every call it made
// was a read.
func assertNoWrite(t *testing.T, stub *platformStub) {
	t.Helper()
	for _, call := range stub.paths {
		if !strings.HasPrefix(call, "GET ") {
			t.Errorf("a refused command made the write %s", call)
		}
	}
}
