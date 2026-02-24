package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	x402v1alpha1 "github.com/razvanmacovei/x402-k8s-operator/api/v1alpha1"
	"github.com/razvanmacovei/x402-k8s-operator/internal/controller"
	"github.com/razvanmacovei/x402-k8s-operator/internal/gateway"
	_ "github.com/razvanmacovei/x402-k8s-operator/internal/metrics"
	"github.com/razvanmacovei/x402-k8s-operator/internal/routestore"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(x402v1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var gatewayAddr string
	var enableLeaderElection bool
	var operatorNamespace string
	var operatorSvcName string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metrics endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&gatewayAddr, "gateway-bind-address", ":8402", "The address the gateway proxy binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")
	flag.StringVar(&operatorNamespace, "operator-namespace", envOrDefault("POD_NAMESPACE", "x402-system"), "Namespace where the operator runs.")
	flag.StringVar(&operatorSvcName, "operator-service-name", envOrDefault("OPERATOR_SERVICE_NAME", "x402-k8s-operator"), "Service name of the operator.")

	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Create shared route store.
	store := routestore.New()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "x402-operator.x402.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Register controller.
	if err = (&controller.X402RouteReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		RouteStore:        store,
		OperatorNamespace: operatorNamespace,
		OperatorSvcName:   operatorSvcName,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "X402Route")
		os.Exit(1)
	}

	// Register gateway as a managed runnable.
	gw := gateway.NewServer(gatewayAddr, store)
	if err := mgr.Add(gw); err != nil {
		setupLog.Error(err, "unable to add gateway server to manager")
		os.Exit(1)
	}

	// Health checks.
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager",
		"metrics", metricsAddr,
		"probes", probeAddr,
		"gateway", gatewayAddr,
		"operatorNamespace", operatorNamespace,
		"operatorSvcName", operatorSvcName,
	)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
