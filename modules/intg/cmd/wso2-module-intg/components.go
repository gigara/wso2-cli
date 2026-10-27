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
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wso2/wso2-cli/modules/intg/internal/platform"
	"github.com/wso2/wso2-cli/sdk/module"
	"github.com/wso2/wso2-cli/sdk/problem"
	"github.com/wso2/wso2-cli/sdk/result"
)

// ComponentsSchema identifies the component listing.
const ComponentsSchema = "intg.components/v1"

// ComponentSchema identifies one component, as component show reports it.
const ComponentSchema = "intg.component/v1"

// CreatedComponentSchema identifies what component create reports: the
// component as created, which can carry a name the platform chose and a
// warning about a step after the create.
const CreatedComponentSchema = "intg.createdComponent/v1"

// ComponentRemovalSchema identifies an accepted component delete.
const ComponentRemovalSchema = "intg.componentRemoval/v1"

// componentTypes maps each --type to the platform component type it creates.
// The names and the mapping are the Integration Platform's own, from the
// create_integration tool of its MCP server, so the CLI, the MCP tools and
// the console say the same thing.
var componentTypes = map[string]string{
	"integration-as-api": "deployment/integration-as-api",
	"automation":         "cronjob/scheduled-task",
	"event-integration":  "deployment/event-integration",
	"file-integration":   "deployment/file-integration",
	"mcp-server":         "deployment/mcp-server",
}

// componentRuntimes maps each --runtime to the build workflow that builds it,
// from the same tool.
var componentRuntimes = map[string]string{
	"wso2-integrator":    "ballerina-buildpack-builder",
	"wso2-integrator-mi": "mi-buildpack-builder",
	"dockerfile":         "dockerfile-builder",
}

// reportedTypes maps the logical type the API reports back to the --type
// word, so a listing reads in the terms create takes. Types create does not
// offer keep a hyphenated form of their own name.
var reportedTypes = map[string]string{
	"service":          "integration-as-api",
	"automation":       "automation",
	"eventIntegration": "event-integration",
	"fileIntegration":  "file-integration",
	"mcpServer":        "mcp-server",
	"aiAgent":          "ai-agent",
	"proxy":            "proxy",
}

// reportedRuntimes maps the buildpack type the API reports to the --runtime
// word. The API reports a Dockerfile build, and any other non-buildpack one,
// as "other".
var reportedRuntimes = map[string]string{
	"BI":    "wso2-integrator",
	"MI":    "wso2-integrator-mi",
	"other": "other",
}

// componentName is what the platform accepts as a component's resource name:
// lowercase letters, digits and hyphens, at most 63 characters.
var componentName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// component is the part of the API's Component this module reports. id is
// the resource name every other command takes; name is the display name.
type component struct {
	Project     string `json:"projectId"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"componentType"`
	Buildpack   string `json:"buildpackType"`
	Prebuilt    bool   `json:"isPrebuilt"`
	Status      string `json:"status"`
	Updated     string `json:"lastBuildDate"`
	Created     string `json:"createdAt"`
	Deleting    bool   `json:"deleting"`
	Warning     string `json:"warning"`
}

func (c component) kind() string {
	if word, known := reportedTypes[c.Type]; known {
		return word
	}
	return orDash(c.Type)
}

func (c component) runtime() string {
	if word, known := reportedRuntimes[c.Buildpack]; known {
		return word
	}
	if c.Prebuilt {
		return "prebuilt"
	}
	return orDash(c.Buildpack)
}

func (c component) state() string {
	if c.Deleting {
		return "deleting"
	}
	return "active"
}

// componentRepository is the part of the API's ComponentRepository this
// module reports.
type componentRepository struct {
	Provider     string `json:"gitProvider"`
	Organization string `json:"organizationApp"`
	Name         string `json:"nameApp"`
	Branch       string `json:"branch"`
	Path         string `json:"appSubPath"`
	URL          string `json:"serverUrl"`
}

func (r componentRepository) describe() string {
	switch {
	case r.Name != "" && r.Organization != "":
		qualified := r.Organization + "/" + r.Name
		if r.Provider != "" {
			return r.Provider + ": " + qualified
		}
		return qualified
	case r.URL != "":
		return r.URL
	default:
		return "-"
	}
}

func projectComponentsPath(projectID string) string {
	return projectPath(projectID) + "/components"
}

func componentPath(id string) string {
	return "/components/" + url.PathEscape(id)
}

// withProject adds projectName to a path when a project is known.
func withProject(path, projectID string) string {
	if projectID == "" {
		return path
	}
	return path + "?" + url.Values{"projectName": {projectID}}.Encode()
}

// componentListRecovery is where to find a component's ID.
func componentListRecovery(projectID string) string {
	if projectID == "" {
		return "Run wso2 intg component list to see the IDs of this organization's components."
	}
	return "Run wso2 intg component list --project " + projectID + " to see the IDs of its components."
}

// listProjects reads the organization's projects.
func listProjects(ctx context.Context, client platform.Client) ([]project, error) {
	var listing struct {
		Items []project `json:"items"`
	}
	err := client.Get(ctx, "/projects", &listing)
	return listing.Items, err
}

// componentsOf reads one project's components, or every project's in turn
// when projectID is empty. The API lists one project at a time, at most 100
// components each, with no continuation cursor.
func componentsOf(ctx context.Context, client platform.Client, projectID, endpoint string) ([]component, error) {
	projectIDs := []string{projectID}
	if projectID == "" {
		projects, err := listProjects(ctx, client)
		if err != nil {
			return nil, callFailed(err, "list the projects", endpoint)
		}
		projectIDs = projectIDs[:0]
		for _, p := range projects {
			projectIDs = append(projectIDs, p.ID)
		}
	}
	var all []component
	for _, id := range projectIDs {
		var listing struct {
			Items []component `json:"items"`
		}
		if err := client.Get(ctx, projectComponentsPath(id), &listing); err != nil {
			return nil, callFailedNaming(err, "list the components of the project "+id, endpoint,
				projectNotFoundRecovery)
		}
		for _, c := range listing.Items {
			if c.Project == "" {
				c.Project = id
			}
			all = append(all, c)
		}
	}
	return all, nil
}

// projectFlag is the --project every component command takes.
type projectFlag struct {
	project string
}

func (f *projectFlag) bind(command *cobra.Command, usage string) {
	command.Flags().StringVar(&f.project, "project", "", usage)
}

// componentList answers "wso2 intg component list [--project <project>]".
func componentList(command *cobra.Command, flags *projectFlag) module.Handler {
	return func(ctx context.Context, request module.Request) (result.Result, error) {
		if err := noArguments(command.Flags().Args(), "Run wso2 intg component list [--project <project>]."); err != nil {
			return result.Result{}, err
		}
		client, err := clientFor(ctx, request)
		if err != nil {
			return result.Result{}, err
		}
		found, err := componentsOf(ctx, client, flags.project, request.Context.Endpoint)
		if err != nil {
			return result.Result{}, err
		}
		report := result.New(ComponentsSchema).
			With("count", "Components", strconv.Itoa(len(found))).
			WithColumn("name", "Name").
			WithColumn("id", "ID").
			WithColumn("project", "Project").
			WithColumn("type", "Type").
			WithColumn("runtime", "Runtime").
			WithColumn("status", "Status").
			WithColumn("updated", "Updated").
			WithColumn("state", "State")
		for _, c := range found {
			report = report.WithRow(c.Name, c.ID, c.Project, c.kind(), c.runtime(), orDash(c.Status),
				orDash(c.Updated), c.state())
		}
		next := "Run wso2 intg component show <id> to see one component."
		if len(found) == 0 {
			next = "There are no components yet. Create one with wso2 intg component create <name> --project <project>."
		}
		return report.With(NextField, "Next", next), nil
	}
}

// findComponent picks the component ref names out of found: by ID first,
// since only IDs are unique, then by display name, refusing a display name
// that more than one component carries.
func findComponent(found []component, ref, projectID string) (component, error) {
	for _, c := range found {
		if c.ID == ref {
			return c, nil
		}
	}
	var named []component
	for _, c := range found {
		if c.Name == ref {
			named = append(named, c)
		}
	}
	switch len(named) {
	case 1:
		return named[0], nil
	case 0:
		where := "in this organization"
		if projectID != "" {
			where = "in the project " + projectID
		}
		return component{}, problem.New(problem.CategoryProductService, "intg.not_found",
			"no component "+where+" has the ID or name "+ref).
			WithRecovery(componentListRecovery(projectID))
	default:
		ids := make([]string, len(named))
		for i, c := range named {
			ids[i] = c.ID
		}
		return component{}, usageProblem("more than one component is named "+ref+": "+strings.Join(ids, ", "),
			"Name the component by its ID instead.")
	}
}

// componentShow answers "wso2 intg component show <component> [--project <project>]".
//
// The API has no read of one component, so the component is found in its
// project's listing, or in every project's when none is named, and its
// repository is read beside it.
func componentShow(command *cobra.Command, flags *projectFlag) module.Handler {
	return func(ctx context.Context, request module.Request) (result.Result, error) {
		ref, err := exactlyOneArgument(command.Flags().Args(), "name the component to show",
			"Run wso2 intg component show <component> [--project <project>].")
		if err != nil {
			return result.Result{}, err
		}
		client, err := clientFor(ctx, request)
		if err != nil {
			return result.Result{}, err
		}
		found, err := componentsOf(ctx, client, flags.project, request.Context.Endpoint)
		if err != nil {
			return result.Result{}, err
		}
		c, err := findComponent(found, ref, flags.project)
		if err != nil {
			return result.Result{}, err
		}
		var repository componentRepository
		if err := client.Get(ctx, withProject(componentPath(c.ID)+"/repository", c.Project), &repository); err != nil {
			return result.Result{}, callFailedNaming(err, "read the repository of the component "+c.ID,
				request.Context.Endpoint, componentListRecovery(c.Project))
		}
		next := "Run wso2 intg component list --project " + c.Project + " to see the other components in its project."
		if c.Deleting {
			next = "The component is being deleted. Run wso2 intg component list --project " + c.Project +
				" until it no longer appears."
		}
		return result.New(ComponentSchema).
			With("id", "ID", c.ID).
			With("name", "Name", c.Name).
			With("project", "Project", c.Project).
			With("type", "Type", c.kind()).
			With("runtime", "Runtime", c.runtime()).
			With("description", "Description", orDash(c.Description)).
			With("status", "Status", orDash(c.Status)).
			With("repository", "Repository", repository.describe()).
			With("branch", "Branch", orDash(repository.Branch)).
			With("path", "Path", orDash(strings.Trim(repository.Path, "/"))).
			With("updated", "Updated", orDash(c.Updated)).
			With("created", "Created", orDash(c.Created)).
			With("state", "State", c.state()).
			With(NextField, "Next", next), nil
	}
}

// componentCreateFlags are the options of "wso2 intg component create".
type componentCreateFlags struct {
	project     string
	kind        string
	runtime     string
	repo        string
	branch      string
	path        string
	displayName string
	description string
	autoDeploy  bool
}

func (f *componentCreateFlags) bind(command *cobra.Command) {
	flags := command.Flags()
	flags.StringVar(&f.project, "project", "", "The project to create it in. Required.")
	flags.StringVar(&f.kind, "type", "", "What it is: "+strings.Join(slices.Sorted(maps.Keys(componentTypes)), ", ")+
		". Required.")
	flags.StringVar(&f.runtime, "runtime", "", "What its source is built with: "+
		strings.Join(slices.Sorted(maps.Keys(componentRuntimes)), ", ")+". Required.")
	flags.StringVar(&f.repo, "repo", "", "The https clone URL of a public git repository. Required.")
	flags.StringVar(&f.branch, "branch", "", "The branch to build. Required.")
	flags.StringVar(&f.path, "path", "", "The directory of the integration within the repository. "+
		"Defaults to the repository root.")
	flags.StringVar(&f.displayName, "display-name", "", "The name people see. Defaults to the component's name.")
	flags.StringVar(&f.description, "description", "", "What the component does.")
	flags.BoolVar(&f.autoDeploy, "auto-deploy", false, "Deploy to the first environment after every successful build.")
}

// createRequest builds the platform-shaped body the API forwards upstream,
// exactly as the platform's own create_integration tool does. autoBuild is
// always set: it starts the first build, without which a new component has
// nothing to deploy.
func (f *componentCreateFlags) createRequest(name string) (map[string]any, error) {
	recovery := "Run wso2 intg component create <name> --project <project> --type <type> --runtime <runtime> " +
		"--repo <url> --branch <branch>."
	if !componentName.MatchString(name) {
		return nil, usageProblem("the component name "+name+" is not one the platform accepts: use lowercase "+
			"letters, digits and hyphens, at most 63 characters", recovery)
	}
	// In a fixed order, so the same command line always names the same flag.
	for _, required := range []struct{ flag, value string }{
		{"--project", f.project}, {"--type", f.kind}, {"--runtime", f.runtime}, {"--repo", f.repo},
		{"--branch", f.branch},
	} {
		if required.value == "" {
			return nil, usageProblem(required.flag+" is required", recovery)
		}
	}
	componentType, known := componentTypes[f.kind]
	if !known {
		return nil, usageProblem("--type "+f.kind+" is not a component type", "Use one of "+
			strings.Join(slices.Sorted(maps.Keys(componentTypes)), ", ")+".")
	}
	workflow, known := componentRuntimes[f.runtime]
	if !known {
		return nil, usageProblem("--runtime "+f.runtime+" is not a runtime", "Use one of "+
			strings.Join(slices.Sorted(maps.Keys(componentRuntimes)), ", ")+".")
	}
	if !strings.HasPrefix(f.repo, "https://") {
		return nil, usageProblem("--repo "+f.repo+" is not an https clone URL",
			"Give the repository's https clone URL, such as https://github.com/<organization>/<repository>.git.")
	}
	annotations := map[string]string{}
	if f.displayName != "" {
		annotations["openchoreo.dev/display-name"] = f.displayName
	}
	if f.description != "" {
		annotations["openchoreo.dev/description"] = f.description
	}
	repository := map[string]any{"url": f.repo, "revision": map[string]string{"branch": f.branch}}
	if f.path != "" {
		repository["appPath"] = f.path
	}
	return map[string]any{
		"metadata": map[string]any{"name": name, "annotations": annotations},
		"spec": map[string]any{
			"owner":         map[string]string{"projectName": f.project},
			"componentType": map[string]string{"name": componentType},
			"autoBuild":     true,
			"autoDeploy":    f.autoDeploy,
			"workflow": map[string]any{
				"kind":       "ClusterWorkflow",
				"name":       workflow,
				"parameters": map[string]any{"repository": repository},
			},
		},
	}, nil
}

// componentCreate answers "wso2 intg component create <name> ...".
func componentCreate(command *cobra.Command, flags *componentCreateFlags) module.Handler {
	return func(ctx context.Context, request module.Request) (result.Result, error) {
		name, err := exactlyOneArgument(command.Flags().Args(), "name the component to create",
			"Run wso2 intg component create <name> --project <project> --type <type> --runtime <runtime> "+
				"--repo <url> --branch <branch>.")
		if err != nil {
			return result.Result{}, err
		}
		body, err := flags.createRequest(name)
		if err != nil {
			return result.Result{}, err
		}
		client, err := clientFor(ctx, request)
		if err != nil {
			return result.Result{}, err
		}
		var created component
		if err := client.Post(ctx, projectComponentsPath(flags.project), body, &created); err != nil {
			return result.Result{}, componentCreateFailed(err, name, request.Context.Endpoint)
		}
		if created.ID == "" {
			created.ID = name
		}
		if created.Project == "" {
			created.Project = flags.project
		}
		// Component names are unique across the organization, and the
		// platform suffixes a taken one rather than refusing it.
		next := "Its first build has started. Run wso2 intg component show " + created.ID + " --project " +
			created.Project + " to follow it."
		if created.ID != name {
			next = "The platform created it as " + created.ID + " because " + name + " was taken. " + next
		}
		return result.New(CreatedComponentSchema).
			With("id", "ID", created.ID).
			With("name", "Name", orDash(created.Name)).
			With("project", "Project", created.Project).
			With("type", "Type", created.kind()).
			With("runtime", "Runtime", created.runtime()).
			With("warning", "Warning", orDash(created.Warning)).
			With(NextField, "Next", next), nil
	}
}

// componentCreateFailed maps the refusals that mean something particular for a
// create, and hands every other one to callFailed.
func componentCreateFailed(err error, name, endpoint string) error {
	if refused, ok := refusal(err, http.StatusConflict); ok {
		return problem.New(problem.CategoryProductService, "intg.component_exists",
			"the platform could not find a free name for the component "+name+": "+refusalWords(refused)).
			WithRecovery("Choose another name and run wso2 intg component create again.")
	}
	if refused, ok := refusal(err, http.StatusBadRequest); ok {
		return problem.New(problem.CategoryProductService, "intg.call_failed",
			"the Integration Platform refused to create the component "+name+": "+refusalWords(refused)).
			WithRecovery("Check the values given on the command line, then run wso2 intg component create again.")
	}
	return callFailed(err, "create the component "+name, endpoint)
}

// componentDeleteFlags are the options of "wso2 intg component delete".
type componentDeleteFlags struct {
	projectFlag
	removalFlags
}

// componentDelete answers "wso2 intg component delete <component> --yes".
//
// The project is optional: the API resolves it from the component, whose
// name is unique across the organization.
func componentDelete(command *cobra.Command, flags *componentDeleteFlags) module.Handler {
	return func(ctx context.Context, request module.Request) (result.Result, error) {
		id, err := exactlyOneArgument(command.Flags().Args(), "name the component to delete",
			"Run wso2 intg component delete <component> --yes.")
		if err != nil {
			return result.Result{}, err
		}
		if !flags.yes {
			return result.Result{}, usageProblem("deleting the component "+id+" is not confirmed",
				"Run wso2 intg component delete "+id+" --yes to delete it.")
		}
		client, err := clientFor(ctx, request)
		if err != nil {
			return result.Result{}, err
		}
		if err := client.Delete(ctx, withProject(componentPath(id), flags.project)); err != nil {
			return result.Result{}, callFailedNaming(err, "delete the component "+id, request.Context.Endpoint,
				componentListRecovery(flags.project))
		}
		return result.New(ComponentRemovalSchema).
			With("component", "Component", id).
			With("state", "State", "deleting").
			With(NextField, "Next", "Deletion continues in the background. Run wso2 intg component list "+
				"until the component no longer appears."), nil
	}
}
