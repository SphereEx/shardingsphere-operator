/*
Copyright 2022.

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

package controllers

import (
	"context"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sphere-ex.com/shardingsphere-operator/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logger "sigs.k8s.io/controller-runtime/pkg/log"

	shardingspherev1alpha1 "sphere-ex.com/shardingsphere-operator/api/v1alpha1"
)

// ProxyConfigReconciler reconciles a ProxyConfig object
type ProxyConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=shardingsphere.sphere-ex.com,resources=proxyconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=shardingsphere.sphere-ex.com,resources=proxyconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=shardingsphere.sphere-ex.com,resources=proxyconfigs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ProxyConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *ProxyConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logger.FromContext(ctx)

	run := &shardingspherev1alpha1.ProxyConfig{}
	err := r.Get(ctx, req.NamespacedName, run)
	if apierrors.IsNotFound(err) {
		log.Error(err, "ProxyConfig in work queue no longer exists!")
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Get CRD Resource Error")
		return ctrl.Result{RequeueAfter: SyncBuildStatusInterval}, err
	}

	cm := &v1.ConfigMap{}
	configmap := reconcile.ConstructCascadingConfigmap(run)
	err = r.Get(ctx, req.NamespacedName, cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Create the configmap")
			err = r.Create(ctx, configmap)
			if err != nil {
				log.Error(err, "Create Configmap Resource Error")
				return ctrl.Result{RequeueAfter: SyncBuildStatusInterval}, err
			}
			run.SetMetadataRepository(run.Spec.ClusterConfig.Repository.Type)
			err = r.Status().Update(ctx, run)
			if err != nil {
				log.Error(err, "Update CRD Resource Status Error")
				return ctrl.Result{RequeueAfter: SyncBuildStatusInterval}, err
			}
			return ctrl.Result{}, nil
		} else {
			log.Error(err, "Get Configmap Resource Error")
			return ctrl.Result{RequeueAfter: SyncBuildStatusInterval}, err
		}
	}
	if !equality.Semantic.DeepEqual(configmap.Data, cm.Data) {
		cm = configmap
		log.Info("Update or correct the configmap")
		err = r.Update(ctx, configmap)
		if err != nil {
			log.Error(err, "Update Configmap Resource Error")
			return ctrl.Result{RequeueAfter: SyncBuildStatusInterval}, err
		}
	}
	if run.Status.MetadataRepository != run.Spec.ClusterConfig.Repository.Type || run.Status.MetadataRepository == "" {
		run.SetMetadataRepository(run.Spec.ClusterConfig.Repository.Type)
		err = r.Status().Update(ctx, run)
		if err != nil {
			log.Error(err, "Update CRD Resource Status Error")
			return ctrl.Result{RequeueAfter: SyncBuildStatusInterval}, err
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProxyConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&shardingspherev1alpha1.ProxyConfig{}).
		Owns(&v1.ConfigMap{}).
		Complete(r)
}