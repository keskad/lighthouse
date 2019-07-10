/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package trigger

import (
	"encoding/json"
	"testing"

	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/lighthouse/pkg/prow/config"
	"github.com/jenkins-x/lighthouse/pkg/prow/fakegithub"
	"github.com/jenkins-x/lighthouse/pkg/prow/labels"
	"github.com/jenkins-x/lighthouse/pkg/prow/plugins"
	"github.com/sirupsen/logrus"
	clienttesting "k8s.io/client-go/testing"
)

func TestTrusted(t *testing.T) {
	const rando = "random-person"
	const member = "org-member"
	const sister = "trusted-org-member"
	const friend = "repo-collaborator"

	var testcases = []struct {
		name     string
		author   string
		labels   []string
		onlyOrg  bool
		expected bool
	}{
		{
			name:     "trust org member",
			author:   member,
			labels:   []string{},
			expected: true,
		},
		{
			name:     "trust member of other trusted org",
			author:   sister,
			labels:   []string{},
			expected: true,
		},
		{
			name:     "accept random PR with ok-to-test",
			author:   rando,
			labels:   []string{labels.OkToTest},
			expected: true,
		},
		{
			name:     "accept random PR with both labels",
			author:   rando,
			labels:   []string{labels.OkToTest, labels.NeedsOkToTest},
			expected: true,
		},
		{
			name:     "reject random PR with needs-ok-to-test",
			author:   rando,
			labels:   []string{labels.NeedsOkToTest},
			expected: false,
		},
		{
			name:     "reject random PR with no label",
			author:   rando,
			labels:   []string{},
			expected: false,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			g := &fakegithub.FakeClient{
				OrgMembers:    map[string][]string{"kubernetes": {sister}, "kubernetes-incubator": {member, fakegithub.Bot}},
				Collaborators: []string{friend},
				IssueComments: map[int][]*scm.Comment{},
			}
			trigger := plugins.Trigger{
				TrustedOrg:     "kubernetes",
				OnlyOrgMembers: tc.onlyOrg,
			}
			var labels []*scm.Label
			for _, label := range tc.labels {
				labels = append(labels, &scm.Label{
					Name: label,
				})
			}
			_, actual, err := TrustedPullRequest(g, trigger, tc.author, "kubernetes-incubator", "random-repo", 1, labels)
			if err != nil {
				t.Fatalf("Didn't expect error: %s", err)
			}
			if actual != tc.expected {
				t.Errorf("actual result %t != expected %t", actual, tc.expected)
			}
		})
	}
}

func TestHandlePullRequest(t *testing.T) {
	var testcases = []struct {
		name string

		Author        string
		ShouldBuild   bool
		ShouldComment bool
		HasOkToTest   bool
		prLabel       string
		prChanges     bool
		prAction      scm.Action
	}{
		{
			name: "Trusted user open PR should build",

			Author:      "t",
			ShouldBuild: true,
			prAction:    scm.ActionOpen,
		},
		{
			name: "Untrusted user open PR should not build and should comment",

			Author:        "u",
			ShouldBuild:   false,
			ShouldComment: true,
			prAction:      scm.ActionOpen,
		},
		{
			name: "Trusted user reopen PR should build",

			Author:      "t",
			ShouldBuild: true,
			prAction:    scm.ActionReopen,
		},
		{
			name: "Untrusted user reopen PR with ok-to-test should build",

			Author:      "u",
			ShouldBuild: true,
			HasOkToTest: true,
			prAction:    scm.ActionReopen,
		},
		{
			name: "Untrusted user reopen PR without ok-to-test should not build",

			Author:      "u",
			ShouldBuild: false,
			prAction:    scm.ActionReopen,
		},
		{
			name: "Trusted user edit PR with changes should build",

			Author:      "t",
			ShouldBuild: true,
			prChanges:   true,
			prAction:    scm.ActionEdited,
		},
		{
			name: "Trusted user edit PR without changes should not build",

			Author:      "t",
			ShouldBuild: false,
			prAction:    scm.ActionEdited,
		},
		{
			name: "Untrusted user edit PR without changes and without ok-to-test should not build",

			Author:      "u",
			ShouldBuild: false,
			prAction:    scm.ActionEdited,
		},
		{
			name: "Untrusted user edit PR with changes and without ok-to-test should not build",

			Author:      "u",
			ShouldBuild: false,
			prChanges:   true,
			prAction:    scm.ActionEdited,
		},
		{
			name: "Untrusted user edit PR without changes and with ok-to-test should not build",

			Author:      "u",
			ShouldBuild: false,
			HasOkToTest: true,
			prAction:    scm.ActionEdited,
		},
		{
			name: "Untrusted user edit PR with changes and with ok-to-test should build",

			Author:      "u",
			ShouldBuild: true,
			HasOkToTest: true,
			prChanges:   true,
			prAction:    scm.ActionEdited,
		},
		{
			name: "Trusted user sync PR should build",

			Author:      "t",
			ShouldBuild: true,
			prAction:    scm.ActionSync,
		},
		{
			name: "Untrusted user sync PR without ok-to-test should not build",

			Author:      "u",
			ShouldBuild: false,
			prAction:    scm.ActionSync,
		},
		{
			name: "Untrusted user sync PR with ok-to-test should build",

			Author:      "u",
			ShouldBuild: true,
			HasOkToTest: true,
			prAction:    scm.ActionSync,
		},
		{
			name: "Trusted user labeled PR with lgtm should not build",

			Author:      "t",
			ShouldBuild: false,
			prAction:    scm.ActionLabel,
			prLabel:     labels.LGTM,
		},
		{
			name: "Untrusted user labeled PR with lgtm should build",

			Author:      "u",
			ShouldBuild: true,
			prAction:    scm.ActionLabel,
			prLabel:     labels.LGTM,
		},
		{
			name: "Untrusted user labeled PR without lgtm should not build",

			Author:      "u",
			ShouldBuild: false,
			prAction:    scm.ActionLabel,
			prLabel:     "test",
		},
		{
			name: "Trusted user closed PR should not build",

			Author:      "t",
			ShouldBuild: false,
			prAction:    scm.ActionClose,
		},
	}
	for _, tc := range testcases {
		t.Logf("running scenario %q", tc.name)

		g := &fakegithub.FakeClient{
			IssueComments: map[int][]*scm.Comment{},
			OrgMembers:    map[string][]string{"org": {"t"}},
			PullRequests: map[int]*scm.PullRequest{
				0: {
					Number: 0,
					Author: scm.User{Login: tc.Author},
					Base: scm.PullRequestBranch{
						Ref: "master",
						Repo: scm.Repository{
							Namespace: "org",
							Name:      "repo",
						},
					},
				},
			},
		}
		fakePlumberClient := fake.NewSimpleClientset()
		c := Client{
			GitHubClient:  g,
			PlumberClient: fakePlumberClient.ProwV1().PlumberJobs("namespace"),
			Config:        &config.Config{},
			Logger:        logrus.WithField("plugin", PluginName),
		}

		presubmits := map[string][]config.Presubmit{
			"org/repo": {
				{
					JobBase: config.JobBase{
						Name: "jib",
					},
					AlwaysRun: true,
				},
			},
		}
		if err := c.Config.SetPresubmits(presubmits); err != nil {
			t.Fatalf("failed to set presubmits: %v", err)
		}

		if tc.HasOkToTest {
			g.IssueLabelsExisting = append(g.IssueLabelsExisting, issueLabels(labels.OkToTest)...)
		}
		pr := scm.PullRequestHook{
			Action: tc.prAction,
			Label:  scm.Label{Name: tc.prLabel},
			PullRequest: scm.PullRequest{
				Number: 0,
				Author: scm.User{Login: tc.Author},
				Base: scm.PullRequestBranch{
					Ref: "master",
					Repo: scm.Repository{
						Owner:    scm.User{Login: "org"},
						Name:     "repo",
						FullName: "org/repo",
					},
				},
			},
		}
		if tc.prChanges {
			data := []byte(`{"base":{"ref":{"from":"REF"}, "sha":{"from":"SHA"}}}`)
			pr.Changes = (json.RawMessage)(data)
		}
		trigger := plugins.Trigger{
			TrustedOrg:     "org",
			OnlyOrgMembers: true,
		}
		if err := handlePR(c, trigger, pr); err != nil {
			t.Fatalf("Didn't expect error: %s", err)
		}
		var numStarted int
		for _, action := range fakePlumberClient.Actions() {
			switch action.(type) {
			case clienttesting.CreateActionImpl:
				numStarted++
			}
		}
		if numStarted > 0 && !tc.ShouldBuild {
			t.Errorf("Built but should not have: %+v", tc)
		} else if numStarted == 0 && tc.ShouldBuild {
			t.Errorf("Not built but should have: %+v", tc)
		}
		if tc.ShouldComment && len(g.IssueCommentsAdded) == 0 {
			t.Error("Expected comment to github")
		} else if !tc.ShouldComment && len(g.IssueCommentsAdded) > 0 {
			t.Errorf("Expected no comments to github, but got %d", len(g.CreatedStatuses))
		}
	}
}
