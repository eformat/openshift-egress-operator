package namespace

import (
	"context"
	"os"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	networkv1 "github.com/openshift/api/network/v1"

	"github.com/redhat-cop/operator-utils/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_namespace")

const annotationBase = "microsegmentation-operator.redhat-cop.io"
const microsgmentationAnnotation = annotationBase + "/microsegmentation"
const egressIP = annotationBase + "/egress-ip"
const egressCIDR = annotationBase + "/egress-cidr"
const egressHosts = annotationBase + "/egress-hosts"
const controllerName = "namespace-controller"

// Add creates a new Namespace Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNamespace{
		ReconcilerBase: util.NewReconcilerBase(mgr.GetClient(), mgr.GetScheme(), mgr.GetConfig(), mgr.GetRecorder(controllerName)),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {

	// Add netowrkv1 to operator sdk scheme
	if err := networkv1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// FIXME - need to add in egress annotations ?
	isAnnotatedNamespace := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			_, ok := e.ObjectOld.(*corev1.Namespace)
			if !ok {
				return false
			}
			_, ok = e.ObjectNew.(*corev1.Namespace)
			if !ok {
				return false
			}
			oldValueMS, _ := e.MetaOld.GetAnnotations()[microsgmentationAnnotation]
			newValueMS, _ := e.MetaNew.GetAnnotations()[microsgmentationAnnotation]
			oldMS := oldValueMS == "true"
			newMS := newValueMS == "true"

			oldValueEIP, _ := e.MetaOld.GetAnnotations()[egressIP]
			newValueEIP, _ := e.MetaNew.GetAnnotations()[egressIP]
			eipResult := oldValueEIP == newValueEIP

			oldValueCIDR, _ := e.MetaOld.GetAnnotations()[egressCIDR]
			newValueCIDR, _ := e.MetaNew.GetAnnotations()[egressCIDR]
			cidrResult := oldValueCIDR == newValueCIDR

			oldValueHOSTS, _ := e.MetaOld.GetAnnotations()[egressHosts]
			newValueHOSTS, _ := e.MetaNew.GetAnnotations()[egressHosts]
			hostsResult := oldValueHOSTS == newValueHOSTS

			return (oldMS != newMS || !eipResult || !cidrResult || !hostsResult)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			_, ok := e.Object.(*corev1.Namespace)
			if !ok {
				return false
			}
			value, _ := e.Meta.GetAnnotations()[microsgmentationAnnotation]
			return value == "true"
		},
	}

	// Watch for changes to primary resource Namespace
	err = c.Watch(&source.Kind{Type: &corev1.Namespace{}}, &handler.EnqueueRequestForObject{}, isAnnotatedNamespace)
	if err != nil {
		return err
	}

	// FIXME - NetNamespace EgressHosts
	// Watch for changes to secondary resource and requeue the owner Namespace
	// err = c.Watch(&source.Kind{Type: &networking.NetworkPolicy{}}, &handler.EnqueueRequestForOwner{
	// 	IsController: true,
	// 	OwnerType:    &corev1.Namespace{},
	// })
	// if err != nil {
	// 	return err
	// }

	return nil
}

var _ reconcile.Reconciler = &ReconcileNamespace{}

// ReconcileNamespace reconciles a Namespace object
type ReconcileNamespace struct {
	util.ReconcilerBase
}

// Reconcile reads that state of the cluster for a Namespace object and makes changes based on the state read
// and what is in the Namespace.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNamespace) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name, "Request.NamespacedName", request.NamespacedName)
	reqLogger.Info("Reconciling Namespace")
	// Fetch the Namespace instance
	instance := &corev1.Namespace{}
	// Funky NamespacedName stuff here, this should work?
	// err := r.GetClient().Get(context.TODO(), request.NamespacedName, instance)
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: request.NamespacedName.Name}, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// The object is being deleted
	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	// Get Previous NetNamespace
	netns := &networkv1.NetNamespace{}
	err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: request.NamespacedName.Name}, netns)
	if err != nil {
		log.Error(err, "unable to find existing NetNamespace", "NetNamespace", netns)
		return r.manageError(err, instance)
	}

	// Reconcile NetNamespace add egressIP
	if instance.Annotations[microsgmentationAnnotation] == "true" {
		if egressIP, ok := instance.Annotations[egressIP]; ok {
			netnamespace := &networkv1.NetNamespace{
				TypeMeta:   metav1.TypeMeta{APIVersion: "network.openshift.io/v1", Kind: "NetNamespace"},
				ObjectMeta: metav1.ObjectMeta{Name: instance.Name},
				NetName:    instance.Name,
				EgressIPs:  []string{egressIP},
				NetID:      netns.NetID,
			}

			err = r.CreateOrUpdateResource(instance, instance.GetNamespace(), netnamespace)
			if err != nil {
				log.Error(err, "unable to update NetNamespace adding egress", "NetNamespace", netnamespace)
				return r.manageError(err, instance)
			}
		}
	} else {
		// Remove egressIP
		if egressIP, ok := instance.Annotations[egressIP]; ok {
			netnamespace := &networkv1.NetNamespace{
				TypeMeta:   metav1.TypeMeta{APIVersion: "network.openshift.io/v1", Kind: "NetNamespace"},
				ObjectMeta: metav1.ObjectMeta{Name: instance.Name},
				NetName:    instance.Name,
				EgressIPs:  []string{},
				NetID:      netns.NetID,
			}
			err = r.CreateOrUpdateResource(instance, instance.GetNamespace(), netnamespace)
			if err != nil {
				if errors.IsNotFound(err) {
					return reconcile.Result{}, nil
				}
				log.Error(err, "unable to update NetNamespace removing egress "+egressIP, "NetNamespace", netns)
				return r.manageError(err, instance)
			}
		}
	}

	// Reconcile for HostSubnet add CIDR
	if instance.Annotations[microsgmentationAnnotation] == "true" {
		if egressHosts, ok := instance.Annotations[egressHosts]; ok {
			if egressCIDR, ok := instance.Annotations[egressCIDR]; ok {
				listOfEgress := strings.Split(egressHosts, ",")
				for _, egressHost := range listOfEgress {
					// Get Previous HostSubnet
					hostsn := &networkv1.HostSubnet{}
					err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: egressHost}, hostsn)
					if err != nil {
						log.Error(err, "unable to find existing HostSubnet "+egressHost, "HostSubnet", hostsn)
						return r.manageError(err, instance)
					}

					hostSubnet := &networkv1.HostSubnet{
						TypeMeta:    metav1.TypeMeta{APIVersion: "network.openshift.io/v1", Kind: "HostSubnet"},
						ObjectMeta:  metav1.ObjectMeta{Name: egressHost},
						Host:        egressHost,
						EgressCIDRs: []string{egressCIDR},
						Subnet:      hostsn.Subnet,
						HostIP:      hostsn.HostIP,
					}
					err = r.CreateOrUpdateResource(instance, instance.GetNamespace(), hostSubnet)
					if err != nil {
						log.Error(err, "unable to update HostSubnet adding egress "+egressCIDR, "HostSubnet", hostSubnet)
						return r.manageError(err, instance)
					}
				}
			}
		}
	} else {
		if egressHosts, ok := instance.Annotations[egressHosts]; ok {
			if egressCIDR, ok := instance.Annotations[egressCIDR]; ok {
				listOfEgress := strings.Split(egressHosts, ",")
				for _, egressHost := range listOfEgress {
					// Get Previous HostSubnet. Need to wait for kube to have removed netnamespace above
					time.Sleep(1000 * time.Millisecond)
					hostsn := &networkv1.HostSubnet{}
					err = r.GetClient().Get(context.TODO(), types.NamespacedName{Name: egressHost}, hostsn)
					if err != nil {
						log.Error(err, "unable to find existing HostSubnet "+egressHost, "HostSubnet", hostsn)
						return reconcile.Result{}, nil
					}
					// Only remove if no other egress IPs hosted on this node
					if len(hostsn.EgressIPs) == 0 {
						hostSubnet := &networkv1.HostSubnet{
							TypeMeta:    metav1.TypeMeta{APIVersion: "network.openshift.io/v1", Kind: "HostSubnet"},
							ObjectMeta:  metav1.ObjectMeta{Name: egressHost},
							Host:        egressHost,
							EgressCIDRs: []string{},
							Subnet:      hostsn.Subnet,
							HostIP:      hostsn.HostIP,
						}
						err = r.CreateOrUpdateResource(instance, instance.GetNamespace(), hostSubnet)
						if err != nil {
							log.Error(err, "unable to update HostSubnet removing egress "+egressCIDR, "HostSubnet", hostSubnet)
							return r.manageError(err, instance)
						}
					}
				}
			}
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileNamespace) manageError(issue error, instance runtime.Object) (reconcile.Result, error) {
	r.GetRecorder().Event(instance, "Warning", "ProcessingError", issue.Error())
	return reconcile.Result{
		RequeueAfter: time.Minute * 2,
		Requeue:      true,
	}, nil
}
