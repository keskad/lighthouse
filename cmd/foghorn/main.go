package main

import (
	"flag"
	"os"

	lighthousev1alpha1 "github.com/jenkins-x/lighthouse/pkg/apis/lighthouse/v1alpha1"
	"github.com/jenkins-x/lighthouse/pkg/clients"
	"github.com/jenkins-x/lighthouse/pkg/foghorn"
	"github.com/jenkins-x/lighthouse/pkg/logrusutil"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

type options struct {
	namespace       string
	configNamespace string
}

func (o *options) Validate() error {
	return nil
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.StringVar(&o.namespace, "namespace", "", "The namespace to listen in (use empty to listen on all namespaces)")
	fs.StringVar(&o.configNamespace, "config-namespace", "jx", "Namespace where configmaps are placed (default to: jx)")

	err := fs.Parse(args)
	if err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	return o
}

func main() {
	logrusutil.ComponentInit("lighthouse-foghorn")

	scheme := runtime.NewScheme()
	if err := lighthousev1alpha1.AddToScheme(scheme); err != nil {
		logrus.WithError(err).Fatal("Failed to register scheme")
	}

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	if o.configNamespace == "" {
		o.configNamespace = "jx"
	}

	logrus.Infof("Starting Lighthouse Foghorn, will listen in '%s' namespace for LighthouseJobs and in '%s' for configuration", o.namespace, o.configNamespace)

	cfg, err := clients.GetConfig("", "")
	if err != nil {
		logrus.WithError(err).Fatal("Could not create kubeconfig")
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme, Namespace: o.namespace})
	if err != nil {
		logrus.WithError(err).Fatal("Unable to start manager")
	}

	reconciler, err := foghorn.NewLighthouseJobReconciler(mgr.GetClient(), mgr.GetScheme(), o.namespace, o.configNamespace)
	if err != nil {
		logrus.WithError(err).Fatal("Unable to instantiate reconciler")
	}
	if err = reconciler.SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Fatal("Unable to create controller")
	}

	defer reconciler.ConfigMapWatcher.Stop()

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logrus.WithError(err).Fatal("Problem running manager")
	}
}
