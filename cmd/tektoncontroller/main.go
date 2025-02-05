package main

import (
	"flag"
	"os"

	"github.com/jenkins-x/lighthouse/pkg/watcher"

	lighthousev1alpha1 "github.com/jenkins-x/lighthouse/pkg/apis/lighthouse/v1alpha1"
	"github.com/jenkins-x/lighthouse/pkg/clients"
	tektonengine "github.com/jenkins-x/lighthouse/pkg/engines/tekton"
	"github.com/jenkins-x/lighthouse/pkg/interrupts"
	"github.com/jenkins-x/lighthouse/pkg/logrusutil"
	"github.com/sirupsen/logrus"
	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

type options struct {
	namespace         string
	policiesNamespace string
	dashboardURL      string
	dashboardTemplate string
}

func (o *options) Validate() error {
	return nil
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.StringVar(&o.namespace, "namespace", "", "The namespace to listen in")
	fs.StringVar(&o.policiesNamespace, "policies-namespace", "", "Namespace where `kind: LighthousePipelineSecurityPolicy` are stored (use empty to listen in all namespaces)")
	fs.StringVar(&o.dashboardURL, "dashboard-url", "", "The base URL for the Tekton Dashboard to link to for build reports")
	fs.StringVar(&o.dashboardTemplate, "dashboard-template", "", "The template expression for generating the URL to the build report based on the PipelineRun parameters. If not specified defaults to $LIGHTHOUSE_DASHBOARD_TEMPLATE")
	err := fs.Parse(args)
	if err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	return o
}

func main() {
	logrusutil.ComponentInit("lighthouse-tekton-controller")

	scheme := runtime.NewScheme()
	if err := lighthousev1alpha1.AddToScheme(scheme); err != nil {
		logrus.WithError(err).Fatal("Failed to register scheme")
	}
	if err := pipelinev1beta1.AddToScheme(scheme); err != nil {
		logrus.WithError(err).Fatal("Failed to register scheme")
	}

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	cfg, err := clients.GetConfig("", "")
	if err != nil {
		logrus.WithError(err).Fatal("Could not create kubeconfig")
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme, Namespace: o.namespace})
	if err != nil {
		logrus.WithError(err).Fatal("Unable to start manager")
	}

	_, _, lhClient, _, err := clients.GetAPIClients()
	if err != nil {
		logrus.WithError(err).Fatal("Error creating lighthouse clients")
	}
	bpWatcher, err := watcher.NewBreakpointWatcher(lhClient, o.namespace, nil)
	if err != nil {
		logrus.WithError(err).Fatal("Unable to create breakpoint watcher")
	}
	defer bpWatcher.Stop()

	reconciler := tektonengine.NewLighthouseJobReconciler(mgr.GetClient(), mgr.GetAPIReader(), mgr.GetScheme(), lhClient, o.dashboardURL, o.dashboardTemplate, o.namespace, o.policiesNamespace, bpWatcher.GetBreakpoints)
	if err = reconciler.SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Fatal("Unable to create controller")
	}

	defer interrupts.WaitForGracefulShutdown()
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logrus.WithError(err).Fatal("Problem running manager")
	}
}
