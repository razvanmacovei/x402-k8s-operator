package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	x402v1alpha1 "github.com/razvanmacovei/x402-k8s-operator/api/v1alpha1"
	"github.com/razvanmacovei/x402-k8s-operator/internal/metrics"
	"github.com/razvanmacovei/x402-k8s-operator/internal/routestore"
)

const (
	finalizerName   = "x402.io/finalizer"
	externalSvcName = "x402-gateway-proxy"
	gatewayPort     = int32(8402)

	annotationOriginalBackends = "x402.io/original-backends"
	annotationManagedBy        = "x402.io/managed-by"
)

// X402RouteReconciler reconciles an X402Route object.
type X402RouteReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	RouteStore         *routestore.Store
	OperatorNamespace  string // namespace where the operator runs (e.g. "x402-system")
	OperatorSvcName    string // service name of the operator (e.g. "x402-k8s-operator")
}

// +kubebuilder:rbac:groups=x402.io,resources=x402routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=x402.io,resources=x402routes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=x402.io,resources=x402routes/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete

func (r *X402RouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the X402Route instance.
	var route x402v1alpha1.X402Route
	if err := r.Get(ctx, req.NamespacedName, &route); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("X402Route resource not found, likely deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch X402Route")
		return ctrl.Result{}, err
	}

	// Handle deletion with finalizer.
	if !route.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&route, finalizerName) {
			if err := r.cleanupResources(ctx, &route); err != nil {
				logger.Error(err, "failed to clean up resources")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&route, finalizerName)
			if err := r.Update(ctx, &route); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present.
	if !controllerutil.ContainsFinalizer(&route, finalizerName) {
		controllerutil.AddFinalizer(&route, finalizerName)
		if err := r.Update(ctx, &route); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Resolve Ingress namespace.
	ingressNS := route.Spec.IngressRef.Namespace
	if ingressNS == "" {
		ingressNS = route.Namespace
	}

	// Step 1: Fetch referenced Ingress and extract original backends.
	ingress := &networkingv1.Ingress{}
	ingressKey := types.NamespacedName{
		Name:      route.Spec.IngressRef.Name,
		Namespace: ingressNS,
	}
	if err := r.Get(ctx, ingressKey, ingress); err != nil {
		logger.Error(err, "failed to fetch referenced Ingress")
		r.setCondition(&route, "IngressPatched", metav1.ConditionFalse, "IngressNotFound", err.Error())
		r.updateStatus(ctx, &route, false, false, 0)
		return ctrl.Result{}, err
	}

	backends := r.extractBackends(ingress)

	// Step 2: Compile CRD rules into route store.
	compiled, err := r.compileRoute(&route, backends)
	if err != nil {
		logger.Error(err, "failed to compile route rules")
		r.setCondition(&route, "Ready", metav1.ConditionFalse, "CompileError", err.Error())
		r.updateStatus(ctx, &route, false, false, 0)
		return ctrl.Result{}, err
	}

	r.RouteStore.Set(route.Namespace, route.Name, compiled)
	metrics.RouteStoreUpdatesTotal.Inc()
	metrics.ActiveRoutes.Set(float64(r.RouteStore.Count()))

	// Step 3: Ensure ExternalName service for cross-namespace routing.
	if err := r.ensureExternalNameService(ctx, ingressNS); err != nil {
		logger.Error(err, "failed to create ExternalName service")
		r.setCondition(&route, "ExternalServiceReady", metav1.ConditionFalse, "ServiceError", err.Error())
		r.updateStatus(ctx, &route, false, false, len(compiled.Rules))
		return ctrl.Result{}, err
	}

	// Step 4: Patch Ingress — paid paths -> operator service, free paths unchanged.
	if err := r.patchIngress(ctx, &route, ingress); err != nil {
		logger.Error(err, "failed to patch Ingress")
		r.setCondition(&route, "IngressPatched", metav1.ConditionFalse, "PatchError", err.Error())
		r.updateStatus(ctx, &route, false, false, len(compiled.Rules))
		return ctrl.Result{}, err
	}
	r.setCondition(&route, "IngressPatched", metav1.ConditionTrue, "Reconciled", "Ingress patched for payment gating")

	// Step 5: Update status.
	r.setCondition(&route, "Ready", metav1.ConditionTrue, "Reconciled", "Route is active and serving traffic")
	r.updateStatus(ctx, &route, true, true, len(compiled.Rules))

	logger.Info("reconciliation complete",
		"ingress", ingressKey.String(),
		"activeRoutes", len(compiled.Rules),
	)
	return ctrl.Result{}, nil
}

// compileRoute converts CRD route rules into a CompiledRoute for the gateway.
func (r *X402RouteReconciler) compileRoute(route *x402v1alpha1.X402Route, backends map[string]string) (*routestore.CompiledRoute, error) {
	facilitatorURL := route.Spec.Payment.FacilitatorURL
	if facilitatorURL == "" {
		facilitatorURL = "https://x402.org/facilitator"
	}

	compiled := &routestore.CompiledRoute{
		Name:           route.Name,
		Namespace:      route.Namespace,
		Wallet:         route.Spec.Payment.Wallet,
		Network:        route.Spec.Payment.Network,
		FacilitatorURL: facilitatorURL,
		DefaultPrice:   route.Spec.Payment.DefaultPrice,
		Backends:       backends,
	}

	for _, rule := range route.Spec.Routes {
		cr := routestore.CompiledRule{
			Path: rule.Path,
			Free: rule.Free,
			Mode: rule.Mode,
		}

		if cr.Mode == "" {
			cr.Mode = "all-pay"
		}

		// Resolve effective price.
		if rule.Price != "" {
			cr.Price = rule.Price
		} else {
			cr.Price = route.Spec.Payment.DefaultPrice
		}

		// Compile conditions.
		for _, cond := range rule.Conditions {
			re, err := regexp.Compile(cond.Pattern)
			if err != nil {
				return nil, fmt.Errorf("compile condition pattern %q: %w", cond.Pattern, err)
			}
			cr.Conditions = append(cr.Conditions, routestore.CompiledCondition{
				Header:  cond.Header,
				Pattern: re,
				Action:  cond.Action,
			})
		}

		compiled.Rules = append(compiled.Rules, cr)
	}

	return compiled, nil
}

// extractBackends reads original backend info from the Ingress.
func (r *X402RouteReconciler) extractBackends(ingress *networkingv1.Ingress) map[string]string {
	logger := log.Log.WithValues("ingress", ingress.Name, "namespace", ingress.Namespace)

	// Check if we already stored original backends.
	if ingress.Annotations != nil {
		if stored, ok := ingress.Annotations[annotationOriginalBackends]; ok {
			var backends map[string]string
			if err := json.Unmarshal([]byte(stored), &backends); err != nil {
				logger.Error(err, "corrupted original-backends annotation, re-extracting from Ingress rules")
				delete(ingress.Annotations, annotationOriginalBackends)
			} else {
				result := make(map[string]string)
				for path, svcPort := range backends {
					parts := strings.SplitN(svcPort, ":", 2)
					if len(parts) == 2 {
						result[path] = fmt.Sprintf("http://%s.%s.svc.cluster.local:%s", parts[0], ingress.Namespace, parts[1])
					}
				}
				return result
			}
		}
	}

	// Extract from current Ingress rules.
	backends := make(map[string]string)
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, p := range rule.HTTP.Paths {
			if p.Backend.Service != nil {
				svcName := p.Backend.Service.Name
				ns := ingress.Namespace
				port := resolveBackendPort(p.Backend.Service.Port)
				backends[p.Path] = fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", svcName, ns, port)
			}
		}
	}
	return backends
}

// resolveBackendPort returns the port number from an IngressServiceBackendPort.
func resolveBackendPort(port networkingv1.ServiceBackendPort) int32 {
	if port.Number != 0 {
		return port.Number
	}
	if port.Name != "" {
		log.Log.Info("ingress backend uses port name, defaulting to 80", "portName", port.Name)
	}
	return 80
}

// ensureExternalNameService creates an ExternalName Service in the user namespace
// pointing to the operator's own service for cross-namespace Ingress routing.
func (r *X402RouteReconciler) ensureExternalNameService(ctx context.Context, namespace string) error {
	if namespace == r.OperatorNamespace {
		return nil
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalSvcName,
			Namespace: namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = map[string]string{
			"app.kubernetes.io/managed-by": "x402-operator",
		}
		svc.Spec.Type = corev1.ServiceTypeExternalName
		svc.Spec.ExternalName = fmt.Sprintf("%s.%s.svc.cluster.local", r.OperatorSvcName, r.OperatorNamespace)
		svc.Spec.Selector = nil
		svc.Spec.ClusterIP = ""
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:     "http",
				Port:     gatewayPort,
				Protocol: corev1.ProtocolTCP,
			},
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("ensure ExternalName service in %s: %w", namespace, err)
	}

	log.FromContext(ctx).Info("ExternalName service reconciled", "namespace", namespace, "operation", op)
	return nil
}

// patchIngress patches the Ingress to route paid paths through the operator's gateway.
func (r *X402RouteReconciler) patchIngress(ctx context.Context, route *x402v1alpha1.X402Route, ingress *networkingv1.Ingress) error {
	if ingress.Annotations == nil {
		ingress.Annotations = make(map[string]string)
	}

	// Store original backends before patching.
	if _, ok := ingress.Annotations[annotationOriginalBackends]; !ok {
		backends := make(map[string]string)
		for _, rule := range ingress.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, p := range rule.HTTP.Paths {
				if p.Backend.Service != nil {
					svcName := p.Backend.Service.Name
					port := resolveBackendPort(p.Backend.Service.Port)
					backends[p.Path] = fmt.Sprintf("%s:%d", svcName, port)
				}
			}
		}
		data, err := json.Marshal(backends)
		if err != nil {
			return fmt.Errorf("marshal original backends: %w", err)
		}
		ingress.Annotations[annotationOriginalBackends] = string(data)
	}

	ingress.Annotations[annotationManagedBy] = "x402-operator"

	// Determine the gateway service name to use in the Ingress.
	ingressNS := ingress.Namespace
	gatewaySvcName := externalSvcName
	if ingressNS == r.OperatorNamespace {
		gatewaySvcName = r.OperatorSvcName
	}

	// Collect paid paths from route rules.
	paidPaths := r.collectPaidPaths(route)

	// Patch Ingress rules: redirect paid paths to gateway.
	for i := range ingress.Spec.Rules {
		if ingress.Spec.Rules[i].HTTP == nil {
			continue
		}
		for j := range ingress.Spec.Rules[i].HTTP.Paths {
			path := ingress.Spec.Rules[i].HTTP.Paths[j].Path
			if r.pathMatchesPaidRoutes(path, paidPaths) {
				ingress.Spec.Rules[i].HTTP.Paths[j].Backend = networkingv1.IngressBackend{
					Service: &networkingv1.IngressServiceBackend{
						Name: gatewaySvcName,
						Port: networkingv1.ServiceBackendPort{
							Number: gatewayPort,
						},
					},
				}
			}
		}
	}

	if err := r.Update(ctx, ingress); err != nil {
		return fmt.Errorf("update ingress: %w", err)
	}

	log.FromContext(ctx).Info("ingress patched", "name", ingress.Name, "namespace", ingress.Namespace)
	return nil
}

// collectPaidPaths extracts all non-free paths from the route rules.
func (r *X402RouteReconciler) collectPaidPaths(route *x402v1alpha1.X402Route) []string {
	var paths []string
	for _, rule := range route.Spec.Routes {
		if !rule.Free {
			paths = append(paths, rule.Path)
		}
	}
	return paths
}

// pathMatchesPaidRoutes checks if an Ingress path should be routed to the gateway.
func (r *X402RouteReconciler) pathMatchesPaidRoutes(ingressPath string, paidPaths []string) bool {
	cleanIngress := strings.TrimSuffix(ingressPath, "(.*)")
	cleanIngress = strings.TrimRight(cleanIngress, "/")
	if cleanIngress == "" {
		cleanIngress = "/"
	}

	for _, paid := range paidPaths {
		cleanPaid := strings.TrimSuffix(paid, "/**")
		cleanPaid = strings.TrimSuffix(cleanPaid, "/*")
		cleanPaid = strings.TrimRight(cleanPaid, "/")
		if cleanPaid == "" {
			cleanPaid = "/"
		}

		if cleanIngress == cleanPaid {
			return true
		}
		if cleanPaid != "/" && strings.HasPrefix(cleanIngress, cleanPaid+"/") {
			return true
		}
		if ingressPath == paid {
			return true
		}
	}
	return false
}

// restoreIngress restores the Ingress to its original state.
func (r *X402RouteReconciler) restoreIngress(ctx context.Context, route *x402v1alpha1.X402Route) error {
	ingressNS := route.Spec.IngressRef.Namespace
	if ingressNS == "" {
		ingressNS = route.Namespace
	}

	ingress := &networkingv1.Ingress{}
	ingressKey := types.NamespacedName{
		Name:      route.Spec.IngressRef.Name,
		Namespace: ingressNS,
	}
	if err := r.Get(ctx, ingressKey, ingress); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get ingress for restore: %w", err)
	}

	if ingress.Annotations == nil {
		return nil
	}
	stored, ok := ingress.Annotations[annotationOriginalBackends]
	if !ok {
		return nil
	}

	var originalBackends map[string]string
	if err := json.Unmarshal([]byte(stored), &originalBackends); err != nil {
		return fmt.Errorf("unmarshal original backends: %w", err)
	}

	for i := range ingress.Spec.Rules {
		if ingress.Spec.Rules[i].HTTP == nil {
			continue
		}
		for j := range ingress.Spec.Rules[i].HTTP.Paths {
			path := ingress.Spec.Rules[i].HTTP.Paths[j].Path
			if original, ok := originalBackends[path]; ok {
				parts := strings.SplitN(original, ":", 2)
				if len(parts) == 2 {
					svcName := parts[0]
					var port int32 = 80
					if p, err := strconv.ParseInt(parts[1], 10, 32); err == nil {
						port = int32(p)
					}
					ingress.Spec.Rules[i].HTTP.Paths[j].Backend = networkingv1.IngressBackend{
						Service: &networkingv1.IngressServiceBackend{
							Name: svcName,
							Port: networkingv1.ServiceBackendPort{
								Number: port,
							},
						},
					}
				}
			}
		}
	}

	delete(ingress.Annotations, annotationOriginalBackends)
	delete(ingress.Annotations, annotationManagedBy)

	if err := r.Update(ctx, ingress); err != nil {
		return fmt.Errorf("restore ingress: %w", err)
	}

	log.FromContext(ctx).Info("ingress restored", "name", ingress.Name)
	return nil
}

// cleanupResources handles finalizer cleanup.
func (r *X402RouteReconciler) cleanupResources(ctx context.Context, route *x402v1alpha1.X402Route) error {
	logger := log.FromContext(ctx)
	var errs []error

	if err := r.restoreIngress(ctx, route); err != nil {
		logger.Error(err, "failed to restore ingress during cleanup")
		errs = append(errs, fmt.Errorf("restore ingress: %w", err))
	}

	// Remove from route store.
	r.RouteStore.Delete(route.Namespace, route.Name)
	metrics.ActiveRoutes.Set(float64(r.RouteStore.Count()))
	metrics.RouteStoreUpdatesTotal.Inc()

	// Clean up ExternalName service if no other X402Routes use this namespace.
	ingressNS := route.Spec.IngressRef.Namespace
	if ingressNS == "" {
		ingressNS = route.Namespace
	}
	if ingressNS != r.OperatorNamespace {
		if err := r.cleanupExternalNameService(ctx, route, ingressNS); err != nil {
			logger.Error(err, "failed to clean up ExternalName service")
			errs = append(errs, fmt.Errorf("cleanup ExternalName service: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %v", errs)
	}

	logger.Info("finalizer: cleanup complete")
	return nil
}

// cleanupExternalNameService removes the ExternalName service if no other routes need it.
func (r *X402RouteReconciler) cleanupExternalNameService(ctx context.Context, route *x402v1alpha1.X402Route, namespace string) error {
	var routeList x402v1alpha1.X402RouteList
	if err := r.List(ctx, &routeList); err != nil {
		return err
	}

	for _, other := range routeList.Items {
		if other.Name == route.Name && other.Namespace == route.Namespace {
			continue
		}
		otherNS := other.Spec.IngressRef.Namespace
		if otherNS == "" {
			otherNS = other.Namespace
		}
		if otherNS == namespace {
			return nil
		}
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalSvcName,
			Namespace: namespace,
		},
	}
	if err := r.Delete(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *X402RouteReconciler) setCondition(route *x402v1alpha1.X402Route, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&route.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: route.Generation,
		LastTransitionTime: metav1.Now(),
	})
}

func (r *X402RouteReconciler) updateStatus(ctx context.Context, route *x402v1alpha1.X402Route, ingressPatched, ready bool, activeRoutes int) {
	route.Status.IngressPatched = ingressPatched
	route.Status.Ready = ready
	route.Status.ActiveRoutes = activeRoutes

	if err := r.Status().Update(ctx, route); err != nil {
		log.FromContext(ctx).Error(err, "failed to update X402Route status")
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *X402RouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&x402v1alpha1.X402Route{}).
		Watches(&networkingv1.Ingress{}, handler.EnqueueRequestsFromMapFunc(r.ingressToX402Routes)).
		Complete(r)
}

// ingressToX402Routes maps an Ingress event to the X402Route(s) that reference it.
func (r *X402RouteReconciler) ingressToX402Routes(ctx context.Context, obj client.Object) []reconcile.Request {
	ingress, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return nil
	}

	if ingress.Annotations == nil || ingress.Annotations[annotationManagedBy] != "x402-operator" {
		return nil
	}

	var routeList x402v1alpha1.X402RouteList
	if err := r.List(ctx, &routeList); err != nil {
		log.FromContext(ctx).Error(err, "failed to list X402Routes for Ingress watch")
		return nil
	}

	var requests []reconcile.Request
	for _, route := range routeList.Items {
		ingressNS := route.Spec.IngressRef.Namespace
		if ingressNS == "" {
			ingressNS = route.Namespace
		}
		if route.Spec.IngressRef.Name == ingress.Name && ingressNS == ingress.Namespace {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      route.Name,
					Namespace: route.Namespace,
				},
			})
		}
	}
	return requests
}
