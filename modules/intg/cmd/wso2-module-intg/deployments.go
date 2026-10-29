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
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wso2/wso2-cli/modules/intg/internal/platform"
	"github.com/wso2/wso2-cli/sdk/module"
	"github.com/wso2/wso2-cli/sdk/problem"
	"github.com/wso2/wso2-cli/sdk/result"
)

// EnvironmentsSchema identifies the environment listing.
const EnvironmentsSchema = "intg.environments/v1"

// DeploymentsSchema identifies a component's deployment in every environment.
const DeploymentsSchema = "intg.deployments/v1"

// DeploymentSchema identifies a component's deployment in one environment.
const DeploymentSchema = "intg.deployment/v1"

// DeploySchema identifies an accepted deploy.
const DeploySchema = "intg.deploy/v1"

// PromotionSchema identifies an accepted promotion.
const PromotionSchema = "intg.promotion/v1"

// StopSchema identifies an accepted stop.
const StopSchema = "intg.stop/v1"

// RedeploySchema identifies an accepted redeploy.
const RedeploySchema = "intg.redeploy/v1"

// notDeployed is the status of an environment a component has no deployment in.
const notDeployed = "not deployed"

// environment is the part of the API's Environment this module reports. id is
// what the API takes wherever an environment is named.
type environment struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Production bool   `json:"critical"`
	DataPlane  string `json:"dpId"`
}

// deployment is the part of the API's ComponentDeployment this module
// reports. The API answers an environment with no deployment with an empty
// body, which leaves it zero.
type deployment struct {
	Release      string `json:"releaseId"`
	Status       string `json:"deploymentStatusV2"`
	Cron         string `json:"cron"`
	CronTimezone string `json:"cronTimezone"`
	Build        *struct {
		Name       string `json:"buildId"`
		DeployedAt string `json:"deployedAt"`
		Commit     *struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	} `json:"build"`
}

func (d deployment) deployed() bool { return d.Release != "" }

func (d deployment) status() string {
	if !d.deployed() {
		return notDeployed
	}
	return orDash(d.Status)
}

func (d deployment) buildName() string {
	if d.Build == nil {
		return "-"
	}
	return orDash(d.Build.Name)
}

func (d deployment) commit() string {
	if d.Build == nil || d.Build.Commit == nil {
		return "-"
	}
	return orDash(d.Build.Commit.SHA)
}

func (d deployment) deployedAt() string {
	if d.Build == nil {
		return "-"
	}
	return orDash(d.Build.DeployedAt)
}

func (d deployment) schedule() string {
	if d.Cron == "" {
		return "-"
	}
	if d.CronTimezone == "" {
		return d.Cron
	}
	return d.Cron + " (" + d.CronTimezone + ")"
}

func listEnvironments(ctx context.Context, client platform.Client, endpoint string) ([]environment, error) {
	var listing struct {
		Items []environment `json:"items"`
	}
	if err := client.Get(ctx, "/environments", &listing); err != nil {
		return nil, callFailed(err, "list the environments", endpoint)
	}
	return listing.Items, nil
}

// resolveEnvironment finds the environment ref names, by ID and then, ignoring
// case, by ID or display name, so a user may type what the console shows. An
// unknown one is refused naming the IDs there are, before anything changes.
func resolveEnvironment(environments []environment, ref, flag string) (environment, error) {
	for _, e := range environments {
		if e.ID == ref {
			return e, nil
		}
	}
	for _, e := range environments {
		if strings.EqualFold(e.ID, ref) || strings.EqualFold(e.Name, ref) {
			return e, nil
		}
	}
	ids := make([]string, len(environments))
	for i, e := range environments {
		ids[i] = e.ID
	}
	return environment{}, usageProblem(flag+" "+ref+" is not an environment of this organization",
		"Use one of "+strings.Join(ids, ", ")+".")
}

// currentDeployment reads what componentID runs in environmentID.
func currentDeployment(ctx context.Context, client platform.Client, componentID, environmentID, endpoint string) (deployment, error) {
	var found deployment
	path := componentPath(componentID) + "/deployments?" + url.Values{"environmentId": {environmentID}}.Encode()
	if err := client.Get(ctx, path, &found); err != nil {
		return deployment{}, callFailedNaming(err, "read the deployment of "+componentID+" in "+environmentID,
			endpoint, componentListRecovery(""))
	}
	return found, nil
}

// componentProject is the project componentID is in: the one --project names,
// or the one the component listing reports when none is given.
func componentProject(ctx context.Context, client platform.Client, componentID, project, endpoint string) (string, error) {
	if project != "" {
		return project, nil
	}
	found, err := componentsOf(ctx, client, "", endpoint)
	if err != nil {
		return "", err
	}
	c, err := findComponent(found, componentID, "")
	if err != nil {
		return "", err
	}
	return c.Project, nil
}

// pipelineOf reads the project's deployment pipeline and where it starts: the
// promotion source that is never itself a target, as the API picks it for a
// first deploy. Both are empty for a project with no pipeline, and entry is
// empty for a pipeline with no promotion paths: then every environment is
// reached by deploying to it directly.
func pipelineOf(ctx context.Context, client platform.Client, project, endpoint string) (pipelineID, entry string, err error) {
	var p struct {
		Pipeline string `json:"defaultDeploymentPipelineId"`
	}
	if err := client.Get(ctx, projectPath(project), &p); err != nil {
		return "", "", callFailedNaming(err, "read the project "+project, endpoint, projectNotFoundRecovery)
	}
	if p.Pipeline == "" {
		return "", "", nil
	}
	var pipeline struct {
		Paths []struct {
			Source  string   `json:"sourceEnvironment"`
			Targets []string `json:"targetEnvironments"`
		} `json:"promotionPaths"`
	}
	if err := client.Get(ctx, "/deploymentpipelines/"+url.PathEscape(p.Pipeline), &pipeline); err != nil {
		return "", "", callFailed(err, "read the deployment pipeline "+p.Pipeline, endpoint)
	}
	if len(pipeline.Paths) == 0 {
		return p.Pipeline, "", nil
	}
	targets := map[string]bool{}
	for _, path := range pipeline.Paths {
		for _, target := range path.Targets {
			targets[target] = true
		}
	}
	for _, path := range pipeline.Paths {
		if !targets[path.Source] {
			return p.Pipeline, path.Source, nil
		}
	}
	// A cycle has no entry, so declaration order is the only ordering left.
	return p.Pipeline, pipeline.Paths[0].Source, nil
}

// requireFlag refuses a command line that leaves out a required flag.
func requireFlag(value, flag, recovery string) error {
	if value == "" {
		return usageProblem(flag+" is required", recovery)
	}
	return nil
}

// environmentList answers "wso2 intg environment list".
func environmentList(command *cobra.Command) module.Handler {
	return func(ctx context.Context, request module.Request) (result.Result, error) {
		if err := noArguments(command.Flags().Args(), "Run wso2 intg environment list."); err != nil {
			return result.Result{}, err
		}
		client, err := clientFor(ctx, request)
		if err != nil {
			return result.Result{}, err
		}
		environments, err := listEnvironments(ctx, client, request.Context.Endpoint)
		if err != nil {
			return result.Result{}, err
		}
		report := result.New(EnvironmentsSchema).
			With("count", "Environments", strconv.Itoa(len(environments))).
			WithColumn("id", "ID").
			WithColumn("name", "Name").
			WithColumn("production", "Production").
			WithColumn("dataPlane", "Data plane")
		for _, e := range environments {
			production := "no"
			if e.Production {
				production = "yes"
			}
			report = report.WithRow(e.ID, e.Name, production, orDash(e.DataPlane))
		}
		return report.With(NextField, "Next", "Run wso2 intg deployment list <component> to see what runs in each."), nil
	}
}

// deploymentList answers "wso2 intg deployment list <component>": what the
// component runs in every environment, one read per environment.
func deploymentList(command *cobra.Command) module.Handler {
	return func(ctx context.Context, request module.Request) (result.Result, error) {
		componentID, err := exactlyOneArgument(command.Flags().Args(), "name the component whose deployments to list",
			"Run wso2 intg deployment list <component>.")
		if err != nil {
			return result.Result{}, err
		}
		client, err := clientFor(ctx, request)
		if err != nil {
			return result.Result{}, err
		}
		environments, err := listEnvironments(ctx, client, request.Context.Endpoint)
		if err != nil {
			return result.Result{}, err
		}
		report := result.New(DeploymentsSchema).
			With("component", "Component", componentID).
			WithColumn("environment", "Environment").
			WithColumn("status", "Status").
			WithColumn("release", "Release").
			WithColumn("build", "Build").
			WithColumn("commit", "Commit").
			WithColumn("deployed", "Deployed")
		for _, e := range environments {
			d, err := currentDeployment(ctx, client, componentID, e.ID, request.Context.Endpoint)
			if err != nil {
				return result.Result{}, err
			}
			report = report.WithRow(e.ID, d.status(), orDash(d.Release), d.buildName(), d.commit(), d.deployedAt())
		}
		return report.With(NextField, "Next", "Run wso2 intg deployment show "+componentID+
			" --env <environment> to see one environment."), nil
	}
}

// environmentFlag is the --env of the commands that act in one environment.
type environmentFlag struct {
	env string
}

// deploymentShow answers "wso2 intg deployment show <component> --env <environment>".
func deploymentShow(command *cobra.Command, flags *environmentFlag) module.Handler {
	return func(ctx context.Context, request module.Request) (result.Result, error) {
		recovery := "Run wso2 intg deployment show <component> --env <environment>."
		componentID, err := exactlyOneArgument(command.Flags().Args(), "name the component whose deployment to show",
			recovery)
		if err != nil {
			return result.Result{}, err
		}
		if err := requireFlag(flags.env, "--env", recovery); err != nil {
			return result.Result{}, err
		}
		client, err := clientFor(ctx, request)
		if err != nil {
			return result.Result{}, err
		}
		environments, err := listEnvironments(ctx, client, request.Context.Endpoint)
		if err != nil {
			return result.Result{}, err
		}
		env, err := resolveEnvironment(environments, flags.env, "--env")
		if err != nil {
			return result.Result{}, err
		}
		d, err := currentDeployment(ctx, client, componentID, env.ID, request.Context.Endpoint)
		if err != nil {
			return result.Result{}, err
		}
		next := "Run wso2 intg deployment list " + componentID + " to see every environment."
		if !d.deployed() {
			next = "Nothing is deployed there. Run wso2 intg component deploy " + componentID + " --env " + env.ID +
				" if the project's pipeline starts there, or wso2 intg component promote " + componentID +
				" --from <environment> --to " + env.ID + "."
		}
		return result.New(DeploymentSchema).
			With("component", "Component", componentID).
			With("environment", "Environment", env.ID).
			With("status", "Status", d.status()).
			With("release", "Release", orDash(d.Release)).
			With("build", "Build", d.buildName()).
			With("commit", "Commit", d.commit()).
			With("deployed", "Deployed", d.deployedAt()).
			With("schedule", "Schedule", d.schedule()).
			With(NextField, "Next", next), nil
	}
}

// componentDeployFlags are the options of "wso2 intg component deploy".
type componentDeployFlags struct {
	env     string
	build   string
	project string
	waitFlags
}

// componentDeploy answers "wso2 intg component deploy <component> --env <environment>".
//
// A deploy goes where the project's deployment pipeline starts, as the console
// does; later environments are reached by promotion, which is what the
// pipeline exists to enforce. The API itself accepts any environment.
func componentDeploy(command *cobra.Command, flags *componentDeployFlags) module.Handler {
	return func(ctx context.Context, request module.Request) (result.Result, error) {
		recovery := "Run wso2 intg component deploy <component> --env <environment> [--build <build>]."
		componentID, err := exactlyOneArgument(command.Flags().Args(), "name the component to deploy", recovery)
		if err != nil {
			return result.Result{}, err
		}
		if err := requireFlag(flags.env, "--env", recovery); err != nil {
			return result.Result{}, err
		}
		if err := flags.validate(); err != nil {
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
		_, entry, err := pipelineOf(ctx, client, project, endpoint)
		if err != nil {
			return result.Result{}, err
		}
		if entry != "" && entry != env.ID {
			return result.Result{}, problem.New(problem.CategoryUsage, "intg.not_pipeline_entry",
				"the project "+project+"'s deployment pipeline starts at "+entry+", not "+env.ID).
				WithRecovery("Run wso2 intg component deploy " + componentID + " --env " + entry +
					", then wso2 intg component promote " + componentID + " --from " + entry + " --to " + env.ID + ".")
		}
		built := "current image"
		if flags.build != "" {
			// The API falls back to the current image when a build cannot be
			// resolved and still reports success, so the build is checked here.
			var b build
			if err := client.Get(ctx, buildPath(componentID, flags.build), &b); err != nil {
				return result.Result{}, buildReadFailed(err, "read the build "+flags.build+" of "+componentID,
					endpoint, componentID)
			}
			if b.Status != "Succeeded" || b.Image == "" {
				return result.Result{}, problem.New(problem.CategoryUsage, "intg.build_not_deployable",
					"the build "+flags.build+" is "+orDash(b.Status)+" and has no image to deploy").
					WithRecovery("Deploy a build that succeeded: " + buildListRecovery(componentID))
			}
			built = flags.build
		}
		var accepted struct {
			Message string `json:"message"`
		}
		path := withProject(componentPath(componentID)+"/deployments", project)
		// Schedule fields are left out so an automation keeps its schedule:
		// the API leaves omitted release-binding fields untouched.
		body := map[string]string{"environmentId": env.ID, "imageId": flags.build}
		if err := client.Post(ctx, path, body, &accepted); err != nil {
			return result.Result{}, callFailedNaming(err, "deploy "+componentID+" to "+env.ID, endpoint,
				componentListRecovery(project))
		}
		release, status, next := accepted.Message, "requested", "Run wso2 intg deployment show "+componentID+
			" --env "+env.ID+" to see it start."
		if flags.wait {
			running, err := waitForRollout(ctx, client, componentID, env.ID, release, flags.timeout, endpoint)
			if err != nil {
				return result.Result{}, err
			}
			release, status = running.Release, running.Status
			next = "Run wso2 intg component logs " + componentID + " --env " + env.ID + " to see what it does."
		}
		return result.New(DeploySchema).
			With("component", "Component", componentID).
			With("environment", "Environment", env.ID).
			With("build", "Build", built).
			With("release", "Release", orDash(release)).
			With("status", "Status", status).
			With(NextField, "Next", next), nil
	}
}

// componentPromoteFlags are the options of "wso2 intg component promote".
type componentPromoteFlags struct {
	from    string
	to      string
	project string
	waitFlags
}

// componentPromote answers "wso2 intg component promote <component> --from <environment> --to <environment>".
//
// The API wants the source release and the pipeline by ID; both are looked up
// here so a user deals only in names.
func componentPromote(command *cobra.Command, flags *componentPromoteFlags) module.Handler {
	return func(ctx context.Context, request module.Request) (result.Result, error) {
		recovery := "Run wso2 intg component promote <component> --from <environment> --to <environment>."
		componentID, err := exactlyOneArgument(command.Flags().Args(), "name the component to promote", recovery)
		if err != nil {
			return result.Result{}, err
		}
		if err := requireFlag(flags.from, "--from", recovery); err != nil {
			return result.Result{}, err
		}
		if err := requireFlag(flags.to, "--to", recovery); err != nil {
			return result.Result{}, err
		}
		if err := flags.validate(); err != nil {
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
		from, err := resolveEnvironment(environments, flags.from, "--from")
		if err != nil {
			return result.Result{}, err
		}
		to, err := resolveEnvironment(environments, flags.to, "--to")
		if err != nil {
			return result.Result{}, err
		}
		if from.ID == to.ID {
			return result.Result{}, usageProblem("--from and --to both name "+from.ID, recovery)
		}
		project, err := componentProject(ctx, client, componentID, flags.project, endpoint)
		if err != nil {
			return result.Result{}, err
		}
		source, err := currentDeployment(ctx, client, componentID, from.ID, endpoint)
		if err != nil {
			return result.Result{}, err
		}
		if !source.deployed() {
			return result.Result{}, problem.New(problem.CategoryProductService, "intg.not_deployed",
				componentID+" has nothing deployed in "+from.ID+" to promote").
				WithRecovery("Run wso2 intg deployment list " + componentID + " to see where it runs.")
		}
		pipelineID, _, err := pipelineOf(ctx, client, project, endpoint)
		if err != nil {
			return result.Result{}, err
		}
		if pipelineID == "" {
			return result.Result{}, problem.New(problem.CategoryProductService, "intg.no_pipeline",
				"the project "+project+" has no deployment pipeline to promote along").
				WithRecovery("Deploy to " + to.ID + " directly with wso2 intg component deploy " + componentID +
					" --env " + to.ID + ".")
		}
		var accepted struct {
			Message string `json:"message"`
		}
		body := map[string]string{
			"sourceReleaseId":      source.Release,
			"sourceEnvironmentId":  from.ID,
			"targetEnvironmentId":  to.ID,
			"deploymentPipelineId": pipelineID,
		}
		path := withProject(componentPath(componentID)+"/deployments/promote", project)
		if err := client.Post(ctx, path, body, &accepted); err != nil {
			// The API answers a hop the pipeline does not allow with a 403,
			// which here is a pipeline rule rather than a missing permission.
			if refused, ok := refusal(err, http.StatusForbidden); ok {
				return result.Result{}, problem.New(problem.CategoryProductService, "intg.promotion_refused",
					"the Integration Platform refused to promote "+componentID+" from "+from.ID+" to "+to.ID+": "+
						refusalWords(refused)).
					WithRecovery("Check that the project " + project + "'s deployment pipeline promotes " + from.ID +
						" to " + to.ID + ", and that you may promote there.")
			}
			return result.Result{}, callFailedNaming(err, "promote "+componentID+" from "+from.ID+" to "+to.ID,
				endpoint, componentListRecovery(project))
		}
		release := accepted.Message
		if release == "" {
			release = source.Release
		}
		status, next := "requested", "Run wso2 intg deployment show "+componentID+" --env "+to.ID+" to see it start."
		if flags.wait {
			running, err := waitForRollout(ctx, client, componentID, to.ID, release, flags.timeout, endpoint)
			if err != nil {
				return result.Result{}, err
			}
			status = running.Status
			next = "Run wso2 intg component logs " + componentID + " --env " + to.ID + " to see what it does."
		}
		return result.New(PromotionSchema).
			With("component", "Component", componentID).
			With("from", "From", from.ID).
			With("to", "To", to.ID).
			With("release", "Release", release).
			With("status", "Status", status).
			With(NextField, "Next", next), nil
	}
}

// resolvedEnvironment is the first steps every one-environment change shares:
// the command line, the access, and the environment --env names.
func resolvedEnvironment(ctx context.Context, command *cobra.Command, request module.Request, flags *environmentFlag,
	verb string) (string, platform.Client, environment, error) {
	recovery := "Run wso2 intg component " + verb + " <component> --env <environment>."
	componentID, err := exactlyOneArgument(command.Flags().Args(), "name the component to "+verb, recovery)
	if err != nil {
		return "", platform.Client{}, environment{}, err
	}
	if err := requireFlag(flags.env, "--env", recovery); err != nil {
		return "", platform.Client{}, environment{}, err
	}
	client, err := clientFor(ctx, request)
	if err != nil {
		return "", platform.Client{}, environment{}, err
	}
	environments, err := listEnvironments(ctx, client, request.Context.Endpoint)
	if err != nil {
		return "", platform.Client{}, environment{}, err
	}
	env, err := resolveEnvironment(environments, flags.env, "--env")
	return componentID, client, env, err
}

// componentStop answers "wso2 intg component stop <component> --env <environment>".
//
// The component no longer runs there, but its deployment is kept, so a
// redeploy starts it again.
func componentStop(command *cobra.Command, flags *environmentFlag) module.Handler {
	return func(ctx context.Context, request module.Request) (result.Result, error) {
		componentID, client, env, err := resolvedEnvironment(ctx, command, request, flags, "stop")
		if err != nil {
			return result.Result{}, err
		}
		var accepted struct {
			Message string `json:"message"`
		}
		path := componentPath(componentID) + "/deployments/stop"
		if err := client.Put(ctx, path, map[string]string{"environment": env.ID}, &accepted); err != nil {
			return result.Result{}, callFailedNaming(err, "stop "+componentID+" in "+env.ID, request.Context.Endpoint,
				"Run wso2 intg deployment list "+componentID+" to see where it is deployed.")
		}
		return result.New(StopSchema).
			With("component", "Component", componentID).
			With("environment", "Environment", env.ID).
			With("state", "State", "stopped").
			With(NextField, "Next", "Run wso2 intg component redeploy "+componentID+" --env "+env.ID+
				" to start it again."), nil
	}
}

// componentRedeploy answers "wso2 intg component redeploy <component> --env <environment>".
//
// The release already deployed there is kept, started again if it was
// stopped, and its replicas restarted one by one.
func componentRedeploy(command *cobra.Command, flags *environmentFlag) module.Handler {
	return func(ctx context.Context, request module.Request) (result.Result, error) {
		componentID, client, env, err := resolvedEnvironment(ctx, command, request, flags, "redeploy")
		if err != nil {
			return result.Result{}, err
		}
		var accepted struct {
			Message string `json:"message"`
		}
		path := componentPath(componentID) + "/deployments/redeploy?" + url.Values{"environment": {env.ID}}.Encode()
		if err := client.Post(ctx, path, nil, &accepted); err != nil {
			return result.Result{}, callFailedNaming(err, "redeploy "+componentID+" in "+env.ID,
				request.Context.Endpoint, "Run wso2 intg deployment list "+componentID+" to see where it is deployed.")
		}
		return result.New(RedeploySchema).
			With("component", "Component", componentID).
			With("environment", "Environment", env.ID).
			With("message", "Message", orDash(accepted.Message)).
			With(NextField, "Next", "Run wso2 intg deployment show "+componentID+" --env "+env.ID+
				" to see it start."), nil
	}
}
