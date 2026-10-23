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

	"github.com/spf13/cobra"

	"github.com/wso2/wso2-cli/sdk/module"
	"github.com/wso2/wso2-cli/sdk/result"
)

// ProjectsSchema identifies the project listing.
const ProjectsSchema = "intg.projects/v1"

// project is the part of the API's Project this module reports. The API
// addresses projects by resource name, so id is what every other command
// takes, and name is the display name.
type project struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Pipeline        string `json:"defaultDeploymentPipelineId"`
	GitOrganization string `json:"gitOrganization"`
	Repository      string `json:"repository"`
}

// repository names the linked repository as organization/name, or "-" when
// the project links none.
func (p project) repository() string {
	switch {
	case p.GitOrganization != "" && p.Repository != "":
		return p.GitOrganization + "/" + p.Repository
	case p.Repository != "":
		return p.Repository
	default:
		return "-"
	}
}

// projectList answers "wso2 intg project list".
//
// The API returns at most one page of projects for the organization the
// token names and carries no continuation cursor, so there is nothing further
// to read.
func projectList(command *cobra.Command) module.Handler {
	return func(ctx context.Context, request module.Request) (result.Result, error) {
		if err := noArguments(command.Flags().Args(), "Run wso2 intg project list."); err != nil {
			return result.Result{}, err
		}
		client, err := clientFor(ctx, request)
		if err != nil {
			return result.Result{}, err
		}
		var listing struct {
			Items []project `json:"items"`
		}
		if err := client.Get(ctx, "/projects", &listing); err != nil {
			return result.Result{}, callFailed(err, "list the projects", request.Context.Endpoint)
		}

		report := result.New(ProjectsSchema).
			With("count", "Projects", strconv.Itoa(len(listing.Items))).
			WithColumn("name", "Name").
			WithColumn("id", "ID").
			WithColumn("pipeline", "Pipeline").
			WithColumn("repository", "Repository")
		for _, p := range listing.Items {
			report = report.WithRow(p.Name, p.ID, p.Pipeline, p.repository())
		}
		next := "Use a project's ID wherever a command takes --project."
		if len(listing.Items) == 0 {
			next = "This organization has no projects yet. Create one in the Integration Platform console."
		}
		return report.With(NextField, "Next", next), nil
	}
}
