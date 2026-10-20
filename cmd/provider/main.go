// Command provider runs the PurelyMail Crossplane provider.
package main

import (
	"flag"
	"log"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	zaplog "sigs.k8s.io/controller-runtime/pkg/log/zap"

	xpcontroller "github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"

	"github.com/aemendes/crossplane-provider-purelymail/apis"
	"github.com/aemendes/crossplane-provider-purelymail/internal/controller"
)

func main() {
	var (
		debug            = flag.Bool("debug", false, "Run with debug logging.")
		syncPeriod       = flag.Duration("sync", time.Hour, "Controller manager sync period, e.g. 1h.")
		pollInterval     = flag.Duration("poll", 10*time.Minute, "How often an individual resource is checked for drift.")
		leaderElection   = flag.Bool("leader-election", false, "Use leader election for the controller manager.")
		maxReconcileRate = flag.Int("max-reconcile-rate", 10, "Global maximum rate per second at which resources may be checked for drift.")
		healthAddr       = flag.String("health-addr", ":8081", "Address at which to serve the /healthz liveness endpoint.")
	)
	flag.Parse()

	zl := zaplog.New(zaplog.UseDevMode(*debug))
	logr := logging.NewLogrLogger(zl.WithName("provider-purelymail"))
	// Always set the controller-runtime logger, not just in debug mode — otherwise
	// it warns "logger was never set" and silently swallows its own internal errors
	// (watch failures, cache issues) rather than just being quieter.
	ctrl.SetLogger(zl)

	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.Fatalf("cannot get API server rest config: %v", err)
	}

	mgr, err := ctrl.NewManager(ratelimiter.LimitRESTConfig(cfg, *maxReconcileRate), ctrl.Options{
		Cache:                  cache.Options{SyncPeriod: syncPeriod},
		LeaderElection:         *leaderElection,
		LeaderElectionID:       "crossplane-leader-election-provider-purelymail",
		HealthProbeBindAddress: *healthAddr,
	})
	if err != nil {
		log.Fatalf("cannot create controller manager: %v", err)
	}

	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Fatalf("cannot add PurelyMail APIs to scheme: %v", err)
	}

	o := xpcontroller.Options{
		Logger:                  logr,
		GlobalRateLimiter:       ratelimiter.NewGlobal(*maxReconcileRate),
		PollInterval:            *pollInterval,
		MaxConcurrentReconciles: *maxReconcileRate,
	}

	if err := controller.Setup(mgr, o); err != nil {
		log.Fatalf("cannot setup PurelyMail controllers: %v", err)
	}

	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		log.Fatalf("cannot create health check: %v", err)
	}

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Fatalf("cannot start controller manager: %v", err)
	}
}
