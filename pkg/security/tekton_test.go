package security

import (
	"context"
	"github.com/jenkins-x/lighthouse/pkg/client/clientset/versioned/fake"
	"testing"
	"time"

	"github.com/jenkins-x/lighthouse/pkg/apis/lighthouse/v1alpha1"
	"github.com/stretchr/testify/assert"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// When there are no any policies defined
// Then nothing in PipelineRun should be changed
func TestApplySecurityPolicyForTektonPipelineRun_MatchesNoAnyPolicyAsThereAreNoAnyAvailable(t *testing.T) {
	// fake client returns no any policy, so the security module will do nothing
	client := fake.NewSimpleClientset()

	run := tektonv1beta1.PipelineRun{}
	run.Name = "release-pipeline"
	run.Namespace = "jx-damian-keska1"
	run.Spec.Timeout = &metav1.Duration{Duration: time.Hour * 5}
	run.Spec.ServiceAccountName = "mindbox-team1-service-account"

	err := ApplySecurityPolicyForTektonPipelineRun(context.TODO(), client, &run, "some-ns")

	assert.Nil(t, err)

	// check that all values remains as they were - NO POLICY, NO OBJECT MUTATION
	assert.Equal(t, "jx-damian-keska1", run.Namespace)
	assert.Equal(t, "5h0m0s", run.Spec.Timeout.Duration.String())
	assert.Equal(t, "mindbox-team1-service-account", run.Spec.ServiceAccountName)
}

// Given Policy defines MaximumPipelineDuration = 1h
// When Pipeline defines Timeout = 5h
// Then 5h is more than allowed 1h, so 1h is set for a Pipeline
func TestApplySecurityPolicyForTektonPipelineRun_EnforcesDurationFromPolicy(t *testing.T) {
	// policy will enforce only time limit
	policy := v1alpha1.LighthousePipelineSecurityPolicy{}
	policy.Name = "mypolicy"
	policy.Namespace = "some-ns"
	policy.Spec.Enforce.MaximumPipelineDuration = &metav1.Duration{Duration: time.Hour} // 1 hour

	// PipelineRun will be associated with "mypolicy"
	run := tektonv1beta1.PipelineRun{}
	run.Name = "release-pipeline"
	run.Spec.Timeout = &metav1.Duration{Duration: time.Hour * 5} // 5 hours
	run.SetLabels(map[string]string{
		PolicyAnnotationName: "mypolicy", // this attached policy matches our test policy
	})

	client := fake.NewSimpleClientset(&policy)

	// before policy is applied
	assert.Equal(t, "5h0m0s", run.Spec.Timeout.Duration.String())

	_ = ApplySecurityPolicyForTektonPipelineRun(context.TODO(), client, &run, "some-ns")

	// after policy is applied (5h defined in job is > than maximum in policy)
	assert.Equal(t, "1h0m0s", run.Spec.Timeout.Duration.String())
}

// Given Policy allows up to 15 minutes of pipeline execution
// When Pipeline sets max execution time to 9 minutes
// Then Pipeline remains its execution time of 9 minutes
func TestApplySecurityPolicyForTektonPipelineRun_DoesNotEnforceDurationWhenPipelineDoesNotReachMaximumAllowed(t *testing.T) {
	policy := v1alpha1.LighthousePipelineSecurityPolicy{}
	policy.Name = "mypolicy"
	policy.Namespace = "some-ns"
	policy.Spec.Enforce.MaximumPipelineDuration = &metav1.Duration{Duration: time.Minute * 15} // maximum allowed is: 15 minutes

	run := tektonv1beta1.PipelineRun{}
	run.Name = "release-pipeline"
	run.Spec.Timeout = &metav1.Duration{Duration: time.Minute * 9} // set to 9 minutes - less than allowed maximum
	run.SetLabels(map[string]string{
		PolicyAnnotationName: "mypolicy", // matches our test policy
	})

	client := fake.NewSimpleClientset(&policy)

	_ = ApplySecurityPolicyForTektonPipelineRun(context.TODO(), client, &run, "some-ns")

	assert.Equal(t, "9m0s", run.Spec.Timeout.Duration.String(), "assert that pipeline will not have enforced 15 minutes timeout, but will keep 9 minutes because it is lower than 15 minutes")
}

// Given POLICY "mypolicy" has definition to enforce a Service Account
// When POLICY is applied on PipelineRun
// Then a PipelineRun field .Spec.ServiceAccountName equals to POLICY's service account enforcement
func TestApplySecurityPolicyForTektonPipelineRun_EnforcesServiceAccountIfDefinedInPolicy(t *testing.T) {
	policy := v1alpha1.LighthousePipelineSecurityPolicy{}
	policy.Name = "mypolicy"
	policy.Namespace = "some-ns"
	policy.Spec.Enforce.ServiceAccountName = "restricted-access-service-account"

	run := tektonv1beta1.PipelineRun{}
	run.Name = "release-pipeline"
	run.Spec.ServiceAccountName = "tekton-bot"
	run.SetLabels(map[string]string{
		PolicyAnnotationName: "mypolicy", // matches our test policy
	})

	client := fake.NewSimpleClientset(&policy)
	_ = ApplySecurityPolicyForTektonPipelineRun(context.TODO(), client, &run, "some-ns")

	assert.Equal(t, "restricted-access-service-account", run.Spec.ServiceAccountName)
}

// Given we define policy1
// And PipelineRun with referenced OTHER POLICY (that does not exist)
// When we try to apply that OTHER POLICY
// Then we get an error
func TestApplySecurityPolicyForTektonPipelineRun_DoesNotMatchPolicy(t *testing.T) {
	policy := v1alpha1.LighthousePipelineSecurityPolicy{}
	policy.Name = "policy1"
	policy.Namespace = "some-ns"

	run := tektonv1beta1.PipelineRun{}
	run.Name = "release-pipeline"
	run.Spec.ServiceAccountName = "tekton-bot"
	run.SetLabels(map[string]string{
		PolicyAnnotationName: "some-other-policy", // DOES NOT MATCH OUR POLICY
	})

	client := fake.NewSimpleClientset(&policy)
	err := ApplySecurityPolicyForTektonPipelineRun(context.TODO(), client, &run, "some-ns")

	assert.Contains(t, err.Error(), "Cannot find LighthousePipelineSecurityPolicy of name some-other-policy in 'some-ns' namespace")
}
