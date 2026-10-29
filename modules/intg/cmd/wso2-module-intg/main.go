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

// Command wso2-module-intg is the WSO2 CLI product module for the WSO2 Cloud
// Integration Platform.
//
// It is built against the public SDK alone and imports no shell package, which
// is what lets it be released, installed, and updated on its own schedule.
//
// The shell owns rendering. A handler returns semantic fields in presentation
// order and never prints: the same handler answers a table run and a JSON run,
// and the field order here is the order both follow. See
// docs/adr/0003-shell-owned-output.md.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wso2/wso2-cli/sdk/cobratree"
	"github.com/wso2/wso2-cli/sdk/module"
	"github.com/wso2/wso2-cli/sdk/result"
)

// Namespace is the product namespace this module owns. It is the first word of
// every command the module answers.
const Namespace = "intg"

// StatusSchema identifies the semantic shape of this module's status result.
// The shell renders it without interpreting it, so a consumer of JSON output
// can rely on the name to know what the fields mean.
const StatusSchema = "intg.status/v1"

// NextField is the field name the shell renders as a trailing next-step line.
// Every result this module returns ends with it, so a user is never left
// wondering what to run.
const NextField = "next"

// IntegrationAudience is the logical name of the Integration Platform API this
// module asks the shell for access to. It is the same against every
// deployment: a context maps it to whatever the deployment binds tokens to,
// which on WSO2 Cloud is the API's own URL, registered as a ThunderID resource
// server.
const IntegrationAudience = "integration-management"

// moduleVersion is this module's own release version. A release injects it:
//
//	go build -ldflags "-X main.moduleVersion=0.1.0"
//
// It moves independently of the shell, protocol, and SDK versions.
var moduleVersion = "0.0.0-dev"

func main() {
	// Standard output carries protocol frames only. Anything this process wants
	// to say goes to standard error, where the shell captures it as bounded
	// diagnostics. See docs/adr/0002-module-transport.md.
	//
	// The tree is served, not its commands: Serve declares the tree to the
	// shell as well, which is what lets the shell answer --help, name a
	// mistyped command, and parse this module's flags before it is launched.
	if err := commands().Serve(context.Background(), moduleOptions()); err != nil {
		fmt.Fprintf(os.Stderr, "wso2-module-intg: %v\n", err)
		os.Exit(1)
	}
}

// moduleOptions describe this module to the SDK.
//
// The audience here and in module.json must stay equal: the shell intersects a
// runtime request with what the module declared at installation. No scopes are
// declared, because the product is reached by token exchange and authorized by
// the organization claims of the token rather than by scopes.
func moduleOptions() module.Options {
	return module.Options{
		Namespace:     Namespace,
		Version:       moduleVersion,
		AuthAudiences: []string{IntegrationAudience},
	}
}

// commands builds this module's command tree and binds each command to its
// handler.
//
// The tree is an ordinary Cobra tree: commands, flags, and help are declared
// here exactly as they would be in a standalone CLI. What differs is the
// ending, because a handler returns fields instead of printing them.
func commands() *cobratree.Tree {
	root := &cobra.Command{
		Use:   Namespace,
		Short: "Manage integrations on the WSO2 Cloud Integration Platform.",
	}
	statusCommand := &cobra.Command{
		Use:   "status",
		Short: "Report this module's own status and what to run first.",
	}
	root.AddCommand(statusCommand)

	// Positional arguments are counted in the handler, never through
	// cobra.Command.Args: the SDK parses flags but does not run Args
	// validators, so one set here would never be consulted.
	projectCommand := &cobra.Command{
		Use:   "project",
		Short: "Work with the organization's projects.",
	}
	projectListCommand := &cobra.Command{
		Use:   "list",
		Short: "List the projects in the organization.",
	}
	projectShowCommand := &cobra.Command{
		Use:   "show <project>",
		Short: "Show one project.",
	}
	projectCreateCommand := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a project, optionally linked to a repository.",
	}
	createFlags := &projectCreateFlags{}
	createFlags.bind(projectCreateCommand)
	projectDeleteCommand := &cobra.Command{
		Use:   "delete <project>",
		Short: "Delete a project and everything in it.",
	}
	deleteFlags := &removalFlags{}
	projectDeleteCommand.Flags().BoolVar(&deleteFlags.yes, "yes", false,
		"Confirm the deletion. Without it nothing is deleted.")
	projectCommand.AddCommand(projectListCommand, projectShowCommand, projectCreateCommand, projectDeleteCommand)
	root.AddCommand(projectCommand)

	componentCommand := &cobra.Command{
		Use:   "component",
		Short: "Work with the integrations in a project.",
	}
	componentListCommand := &cobra.Command{
		Use:   "list",
		Short: "List the components of a project, or of every project.",
	}
	listFlags := &projectFlag{}
	listFlags.bind(componentListCommand, "The project to list. Defaults to every project.")
	componentShowCommand := &cobra.Command{
		Use:   "show <component>",
		Short: "Show one component and its source repository.",
	}
	showFlags := &projectFlag{}
	showFlags.bind(componentShowCommand, "The project to look in. Defaults to every project.")
	componentCreateCommand := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a component that builds from a public git repository.",
	}
	componentCreateFlagsValue := &componentCreateFlags{}
	componentCreateFlagsValue.bind(componentCreateCommand)
	componentDeleteCommand := &cobra.Command{
		Use:   "delete <component>",
		Short: "Delete a component.",
	}
	componentDeleteFlagsValue := &componentDeleteFlags{}
	componentDeleteFlagsValue.bind(componentDeleteCommand,
		"The project the component is in. Defaults to the one the platform records.")
	componentDeleteCommand.Flags().BoolVar(&componentDeleteFlagsValue.yes, "yes", false,
		"Confirm the deletion. Without it nothing is deleted.")
	componentCommand.AddCommand(componentListCommand, componentShowCommand, componentCreateCommand,
		componentDeleteCommand)
	root.AddCommand(componentCommand)

	buildCommand := &cobra.Command{
		Use:   "build",
		Short: "Start builds of a component and read their status and logs.",
	}
	buildRunCommand := &cobra.Command{
		Use:   "run <component>",
		Short: "Start a build of a component.",
	}
	buildRunFlagsValue := &buildRunFlags{}
	buildRunCommand.Flags().StringVar(&buildRunFlagsValue.commit, "commit", "",
		"The full 40-character commit SHA to build. Defaults to the head of the component's branch.")
	buildRunFlagsValue.bind(buildRunCommand, "the build finishes")
	cobratree.WithTimeout(buildRunCommand, waitDeadline)
	buildListCommand := &cobra.Command{
		Use:   "list <component>",
		Short: "List a component's recent builds, newest first.",
	}
	buildShowCommand := &cobra.Command{
		Use:   "show <component> <build>",
		Short: "Show one build's status, commit, image and steps.",
	}
	buildLogsCommand := &cobra.Command{
		Use:   "logs <component> <build>",
		Short: "Show one build's log.",
	}
	buildLogsFlagsValue := &buildLogsFlags{}
	buildLogsCommand.Flags().IntVar(&buildLogsFlagsValue.tail, "tail", 0,
		"Show only the last this many lines. Defaults to the whole log.")
	buildCommand.AddCommand(buildRunCommand, buildListCommand, buildShowCommand, buildLogsCommand)
	root.AddCommand(buildCommand)

	componentDeployCommand := &cobra.Command{
		Use:   "deploy <component>",
		Short: "Deploy a component where its project's pipeline starts.",
	}
	deployFlags := &componentDeployFlags{}
	componentDeployCommand.Flags().StringVar(&deployFlags.env, "env", "", "The environment to deploy to. Required.")
	componentDeployCommand.Flags().StringVar(&deployFlags.build, "build", "",
		"A succeeded build whose image to deploy. Defaults to the component's current image.")
	componentDeployCommand.Flags().StringVar(&deployFlags.project, "project", "",
		"The project the component is in. Defaults to the one the component listing reports.")
	deployFlags.bind(componentDeployCommand, "the new release is running")
	cobratree.WithTimeout(componentDeployCommand, waitDeadline)
	componentPromoteCommand := &cobra.Command{
		Use:   "promote <component>",
		Short: "Promote what runs in one environment to the next, along the project's pipeline.",
	}
	promoteFlags := &componentPromoteFlags{}
	componentPromoteCommand.Flags().StringVar(&promoteFlags.from, "from", "",
		"The environment whose running release to promote. Required.")
	componentPromoteCommand.Flags().StringVar(&promoteFlags.to, "to", "", "The environment to promote into. Required.")
	componentPromoteCommand.Flags().StringVar(&promoteFlags.project, "project", "",
		"The project the component is in. Defaults to the one the component listing reports.")
	promoteFlags.bind(componentPromoteCommand, "the promoted release is running")
	cobratree.WithTimeout(componentPromoteCommand, waitDeadline)
	componentStopCommand := &cobra.Command{
		Use:   "stop <component>",
		Short: "Stop a component in one environment, keeping its deployment.",
	}
	stopFlags := &environmentFlag{}
	componentStopCommand.Flags().StringVar(&stopFlags.env, "env", "", "The environment to stop it in. Required.")
	componentRedeployCommand := &cobra.Command{
		Use:   "redeploy <component>",
		Short: "Restart what a component runs in one environment, or start it again after a stop.",
	}
	redeployFlags := &environmentFlag{}
	componentRedeployCommand.Flags().StringVar(&redeployFlags.env, "env", "",
		"The environment to redeploy it in. Required.")
	componentLogsCommand := &cobra.Command{
		Use:   "logs <component>",
		Short: "Show what a component logs in one environment.",
	}
	componentLogsFlagsValue := &componentLogsFlags{}
	componentLogsCommand.Flags().StringVar(&componentLogsFlagsValue.env, "env", "",
		"The environment to read its logs in. Required.")
	componentLogsCommand.Flags().StringVar(&componentLogsFlagsValue.since, "since", "1h",
		"How far back to look, such as 15m or 24h. Defaults to 1h.")
	componentLogsCommand.Flags().IntVar(&componentLogsFlagsValue.tail, "tail", defaultLogTail,
		"Show only the newest this many lines, 100 unless given; 0 shows every line in the window, up to 1000.")
	componentLogsCommand.Flags().StringVar(&componentLogsFlagsValue.project, "project", "",
		"The project the component is in. Defaults to the one the component listing reports.")
	componentCommand.AddCommand(componentDeployCommand, componentPromoteCommand, componentStopCommand,
		componentRedeployCommand, componentLogsCommand)

	environmentCommand := &cobra.Command{
		Use:   "environment",
		Short: "See the organization's deployment environments.",
	}
	environmentListCommand := &cobra.Command{
		Use:   "list",
		Short: "List the organization's environments.",
	}
	environmentCommand.AddCommand(environmentListCommand)
	root.AddCommand(environmentCommand)

	deploymentCommand := &cobra.Command{
		Use:   "deployment",
		Short: "See what a component runs in each environment.",
	}
	deploymentListCommand := &cobra.Command{
		Use:   "list <component>",
		Short: "List what a component runs in every environment.",
	}
	deploymentShowCommand := &cobra.Command{
		Use:   "show <component>",
		Short: "Show what a component runs in one environment.",
	}
	showDeploymentFlags := &environmentFlag{}
	deploymentShowCommand.Flags().StringVar(&showDeploymentFlags.env, "env", "", "The environment to show. Required.")
	deploymentCommand.AddCommand(deploymentListCommand, deploymentShowCommand)
	root.AddCommand(deploymentCommand)

	return cobratree.New(root).
		Handle(statusCommand, status).
		Handle(projectListCommand, projectList(projectListCommand)).
		Handle(projectShowCommand, projectShow(projectShowCommand)).
		Handle(projectCreateCommand, projectCreate(projectCreateCommand, createFlags)).
		Handle(projectDeleteCommand, projectDelete(projectDeleteCommand, deleteFlags)).
		Handle(componentListCommand, componentList(componentListCommand, listFlags)).
		Handle(componentShowCommand, componentShow(componentShowCommand, showFlags)).
		Handle(componentCreateCommand, componentCreate(componentCreateCommand, componentCreateFlagsValue)).
		Handle(componentDeleteCommand, componentDelete(componentDeleteCommand, componentDeleteFlagsValue)).
		Handle(buildRunCommand, buildRun(buildRunCommand, buildRunFlagsValue)).
		Handle(buildListCommand, buildList(buildListCommand)).
		Handle(buildShowCommand, buildShow(buildShowCommand)).
		Handle(buildLogsCommand, buildLogs(buildLogsCommand, buildLogsFlagsValue)).
		Handle(componentDeployCommand, componentDeploy(componentDeployCommand, deployFlags)).
		Handle(componentPromoteCommand, componentPromote(componentPromoteCommand, promoteFlags)).
		Handle(componentStopCommand, componentStop(componentStopCommand, stopFlags)).
		Handle(componentRedeployCommand, componentRedeploy(componentRedeployCommand, redeployFlags)).
		Handle(environmentListCommand, environmentList(environmentListCommand)).
		Handle(deploymentListCommand, deploymentList(deploymentListCommand)).
		Handle(deploymentShowCommand, deploymentShow(deploymentShowCommand, showDeploymentFlags)).
		Handle(componentLogsCommand, componentLogs(componentLogsCommand, componentLogsFlagsValue))
}

// status answers "wso2 intg status".
//
// It reports what it can know without asking anything of the shell, so it
// answers before a context records the product or a login exists.
func status(ctx context.Context, request module.Request) (result.Result, error) {
	next := "Record where this product runs on the selected context: " +
		"wso2 context product add intg --url <url>."
	if request.Context.Endpoint != "" {
		next = "Run wso2 intg project list to see the organization's projects at " +
			request.Context.Endpoint + "."
	}
	return result.New(StatusSchema).
		With("namespace", "Namespace", Namespace).
		With("version", "Version", moduleVersion).
		With("endpoint", "Endpoint", request.Context.Endpoint).
		With(NextField, "Next", next), nil
}
