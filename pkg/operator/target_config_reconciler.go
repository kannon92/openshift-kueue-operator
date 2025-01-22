package operator

import (
	"context"
	"fmt"
	"strconv"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	openshiftrouteclientset "github.com/openshift/client-go/route/clientset/versioned"
	"github.com/openshift/kueue-operator/bindata"
	kueuev1alpha1 "github.com/openshift/kueue-operator/pkg/apis/kueueoperator/v1alpha1"
	"github.com/openshift/kueue-operator/pkg/builders/configmap"
	kueueconfigclient "github.com/openshift/kueue-operator/pkg/generated/clientset/versioned/typed/kueueoperator/v1alpha1"
	operatorclientinformers "github.com/openshift/kueue-operator/pkg/generated/informers/externalversions/kueueoperator/v1alpha1"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"

	"github.com/openshift/kueue-operator/pkg/operator/operatorclient"

	"github.com/openshift/library-go/pkg/controller"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	rbacv1 "k8s.io/api/rbac/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

const (
	PromNamespace       = "openshift-monitoring"
	KueueConfigMap      = "kueue-manager-config"
	KueueServiceAccount = "openshift-kueue-operator"
	PromRouteName       = "prometheus-k8s"
	PromTokenPrefix     = "prometheus-k8s-token"
)

type TargetConfigReconciler struct {
	ctx                        context.Context
	operatorClient             kueueconfigclient.KueueV1alpha1Interface
	kueueClient                *operatorclient.KueueClient
	kubeClient                 kubernetes.Interface
	osrClient                  openshiftrouteclientset.Interface
	dynamicClient              dynamic.Interface
	eventRecorder              events.Recorder
	queue                      workqueue.RateLimitingInterface
	kubeInformersForNamespaces v1helpers.KubeInformersForNamespaces
	crdClient                  apiextv1.ApiextensionsV1Interface
}

func NewTargetConfigReconciler(
	ctx context.Context,
	operatorConfigClient kueueconfigclient.KueueV1alpha1Interface,
	operatorClientInformer operatorclientinformers.KueueInformer,
	kubeInformersForNamespaces v1helpers.KubeInformersForNamespaces,
	kueueClient *operatorclient.KueueClient,
	kubeClient kubernetes.Interface,
	osrClient openshiftrouteclientset.Interface,
	dynamicClient dynamic.Interface,
	crdClient apiextv1.ApiextensionsV1Interface,
	eventRecorder events.Recorder,
) (*TargetConfigReconciler, error) {
	c := &TargetConfigReconciler{
		ctx:                        ctx,
		operatorClient:             operatorConfigClient,
		kueueClient:                kueueClient,
		kubeClient:                 kubeClient,
		osrClient:                  osrClient,
		dynamicClient:              dynamicClient,
		eventRecorder:              eventRecorder,
		queue:                      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "TargetConfigReconciler"),
		kubeInformersForNamespaces: kubeInformersForNamespaces,
		crdClient:                  crdClient,
	}

	_, err := operatorClientInformer.Informer().AddEventHandler(c.eventHandler(queueItem{kind: "kueue"}))
	if err != nil {
		return nil, err
	}

	_, err = kubeInformersForNamespaces.InformersFor(operatorclient.OperatorNamespace).Core().V1().ConfigMaps().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {},
		UpdateFunc: func(old, new interface{}) {
			cm, ok := old.(*v1.ConfigMap)
			if !ok {
				klog.Errorf("Unable to convert obj to ConfigMap")
				return
			}
			c.queue.Add(queueItem{kind: "configmap", name: cm.Name})
		},
		DeleteFunc: func(obj interface{}) {
			cm, ok := obj.(*v1.ConfigMap)
			if !ok {
				klog.Errorf("Unable to convert obj to ConfigMap")
				return
			}
			c.queue.Add(queueItem{kind: "configmap", name: cm.Name})
		},
	})
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (c TargetConfigReconciler) sync(item queueItem) error {
	kueue, err := c.operatorClient.Kueues(operatorclient.OperatorNamespace).Get(c.ctx, operatorclient.OperatorConfigName, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "unable to get operator configuration", "namespace", operatorclient.OperatorNamespace, "kueue", operatorclient.OperatorConfigName)
		return err
	}

	specAnnotations := map[string]string{
		"kueueoperator.operator.openshift.io/cluster": strconv.FormatInt(kueue.Generation, 10),
	}

	// Skip any sync triggered by other than the Kueue CM changes
	if item.kind == "configmap" {
		if item.name != KueueConfigMap {
			return nil
		}
		klog.Infof("configmap %q changed, forcing redeployment", KueueConfigMap)
	}

	if cm, _, err := c.manageConfigMap(kueue); err != nil {
		return err
	} else {
		resourceVersion := "0"
		if cm != nil { // SyncConfigMap can return nil
			resourceVersion = cm.ObjectMeta.ResourceVersion
		}
		specAnnotations["kueue/configmap"] = resourceVersion

	}

	crdAnnotations, crdErr := c.manageCustomResources(kueue)

	if crdErr != nil {
		return crdErr
	}

	for key, val := range crdAnnotations {
		specAnnotations[key] = val
	}

	if sa, _, err := c.manageServiceAccount(kueue); err != nil {
		return err
	} else {
		resourceVersion := "0"
		if sa != nil { // SyncConfigMap can return nil
			resourceVersion = sa.ObjectMeta.ResourceVersion
		}
		specAnnotations["serviceaccounts/kueue-operator"] = resourceVersion
	}

	if secret, _, err := c.manageSecret(kueue); err != nil {
		return err
	} else {
		resourceVersion := "0"
		if secret != nil { // SyncConfigMap can return nil
			resourceVersion = secret.ObjectMeta.ResourceVersion
		}
		specAnnotations["secret/kueue-webhook-server-cert"] = resourceVersion
	}

	if roleBindings, _, err := c.manageRole(kueue, "assets/kueue-operator/role-leader-election.yaml"); err != nil {
		return err
	} else {
		resourceVersion := "0"
		if roleBindings != nil { // SyncConfigMap can return nil
			resourceVersion = roleBindings.ObjectMeta.ResourceVersion
		}
		specAnnotations["role/leader-election"] = resourceVersion
	}

	if roleBindings, _, err := c.manageRoleBindings(kueue, "assets/kueue-operator/rolebinding-leader-election.yaml"); err != nil {
		return err
	} else {
		resourceVersion := "0"
		if roleBindings != nil { // SyncConfigMap can return nil
			resourceVersion = roleBindings.ObjectMeta.ResourceVersion
		}
		specAnnotations["rolebindings/leader-election"] = resourceVersion
	}

	if service, _, err := c.manageService(kueue, "assets/kueue-operator/metrics-service.yaml"); err != nil {
		return err
	} else {
		resourceVersion := "0"
		if service != nil { // SyncConfigMap can return nil
			resourceVersion = service.ObjectMeta.ResourceVersion
		}
		specAnnotations["service/metrics-service"] = resourceVersion
	}

	if service, _, err := c.manageService(kueue, "assets/kueue-operator/visibility-service.yaml"); err != nil {
		return err
	} else {
		resourceVersion := "0"
		if service != nil { // SyncConfigMap can return nil
			resourceVersion = service.ObjectMeta.ResourceVersion
		}
		specAnnotations["service/visibility-service"] = resourceVersion
	}

	if service, _, err := c.manageService(kueue, "assets/kueue-operator/webhook-service.yaml"); err != nil {
		return err
	} else {
		resourceVersion := "0"
		if service != nil { // SyncConfigMap can return nil
			resourceVersion = service.ObjectMeta.ResourceVersion
		}
		specAnnotations["service/webhook-service"] = resourceVersion
	}

	annotations, err := c.manageClusterRoles(kueue)
	if err != nil {
		return err
	}
	for key, val := range annotations {
		specAnnotations[key] = val
	}

	if service, _, err := c.manageClusterRoleBindings(kueue, "assets/kueue-operator/clusterrolebinding-kube-proxy.yaml"); err != nil {
		return err
	} else {
		resourceVersion := "0"
		if service != nil { // SyncConfigMap can return nil
			resourceVersion = service.ObjectMeta.ResourceVersion
		}
		specAnnotations["clusterrolebinding/webhook-service"] = resourceVersion
	}

	if service, _, err := c.manageClusterRoleBindings(kueue, "assets/kueue-operator/clusterrolebinding-kueue-manager-role.yaml"); err != nil {
		return err
	} else {
		resourceVersion := "0"
		if service != nil { // SyncConfigMap can return nil
			resourceVersion = service.ObjectMeta.ResourceVersion
		}
		specAnnotations["clusterrolebinding/webhook-service"] = resourceVersion
	}

	deployment, _, err := c.manageDeployment(kueue, specAnnotations)
	if err != nil {
		return err
	}

	_, _, err = v1helpers.UpdateStatus(c.ctx, c.kueueClient, func(status *operatorv1.OperatorStatus) error {
		resourcemerge.SetDeploymentGeneration(&status.Generations, deployment)
		return nil
	})

	if err != nil {
		return err
	}

	if _, _, err := c.manageMutatingWebhook(kueue); err != nil {
		return err
	}

	if _, _, err := c.manageValidatingWebhook(kueue); err != nil {
		return err
	}

	return nil
}

func (c *TargetConfigReconciler) manageConfigMap(kueue *kueuev1alpha1.Kueue) (*v1.ConfigMap, bool, error) {
	required, err := c.kubeClient.CoreV1().ConfigMaps(operatorclient.OperatorNamespace).Get(context.TODO(), KueueConfigMap, metav1.GetOptions{})

	if errors.IsNotFound(err) {
		return c.buildAndApplyConfigMap(nil, kueue.Spec.Config)
	} else if err != nil {
		klog.Errorf("Cannot load ConfigMap %s for the kueue operator", operatorclient.OperatorNamespace)
		return nil, false, err
	}
	return c.buildAndApplyConfigMap(required, kueue.Spec.Config)
}

func (c *TargetConfigReconciler) buildAndApplyConfigMap(oldCfgMap *v1.ConfigMap, kueueCfg kueuev1alpha1.KueueConfiguration) (*v1.ConfigMap, bool, error) {
	cfgMap, buildErr := configmap.BuildConfigMap(operatorclient.OperatorNamespace, kueueCfg)
	if buildErr != nil {
		klog.Errorf("Cannot build configmap %s for kueue", operatorclient.OperatorNamespace)
		return nil, false, buildErr
	}
	if oldCfgMap != nil && oldCfgMap.Data["controller_manager_config.yaml"] == cfgMap.Data["controller_manager_config.yaml"] {
		return nil, true, nil
	}
	klog.InfoS("Configmap difference detected", "Namespace", operatorclient.OperatorNamespace, "ConfigMap", KueueConfigMap)
	return resourceapply.ApplyConfigMap(c.ctx, c.kubeClient.CoreV1(), c.eventRecorder, cfgMap)

}

func (c *TargetConfigReconciler) manageServiceAccount(kueue *kueuev1alpha1.Kueue) (*v1.ServiceAccount, bool, error) {
	required := resourceread.ReadServiceAccountV1OrDie(bindata.MustAsset("assets/kueue-operator/serviceaccount.yaml"))
	required.Namespace = kueue.Namespace
	ownerReference := metav1.OwnerReference{
		APIVersion: "operator.openshift.io/v1alpha1",
		Kind:       "Kueue",
		Name:       kueue.Name,
		UID:        kueue.UID,
	}
	required.OwnerReferences = []metav1.OwnerReference{
		ownerReference,
	}
	controller.EnsureOwnerRef(required, ownerReference)

	return resourceapply.ApplyServiceAccount(c.ctx, c.kubeClient.CoreV1(), c.eventRecorder, required)
}

func (c *TargetConfigReconciler) manageSecret(kueue *kueuev1alpha1.Kueue) (*v1.Secret, bool, error) {
	required := resourceread.ReadSecretV1OrDie(bindata.MustAsset("assets/kueue-operator/secret.yaml"))
	required.Namespace = kueue.Namespace
	ownerReference := metav1.OwnerReference{
		APIVersion: "operator.openshift.io/v1alpha1",
		Kind:       "Kueue",
		Name:       kueue.Name,
		UID:        kueue.UID,
	}
	required.OwnerReferences = []metav1.OwnerReference{
		ownerReference,
	}
	controller.EnsureOwnerRef(required, ownerReference)

	return resourceapply.ApplySecret(c.ctx, c.kubeClient.CoreV1(), c.eventRecorder, required)
}

func (c *TargetConfigReconciler) manageMutatingWebhook(kueue *kueuev1alpha1.Kueue) (*admissionregistrationv1.MutatingWebhookConfiguration, bool, error) {
	required := resourceread.ReadMutatingWebhookConfigurationV1OrDie(bindata.MustAsset("assets/kueue-operator/mutatingwebhook.yaml"))
	ownerReference := metav1.OwnerReference{
		APIVersion: "operator.openshift.io/v1alpha1",
		Kind:       "Kueue",
		Name:       kueue.Name,
		UID:        kueue.UID,
	}
	required.OwnerReferences = []metav1.OwnerReference{
		ownerReference,
	}
	controller.EnsureOwnerRef(required, ownerReference)

	for i := range required.Webhooks {
		required.Webhooks[i].ClientConfig.Service.Namespace = kueue.Namespace
	}
	return resourceapply.ApplyMutatingWebhookConfigurationImproved(c.ctx, c.kubeClient.AdmissionregistrationV1(), c.eventRecorder, required, resourceapply.NewResourceCache())
}

func (c *TargetConfigReconciler) manageValidatingWebhook(kueue *kueuev1alpha1.Kueue) (*admissionregistrationv1.ValidatingWebhookConfiguration, bool, error) {
	required := resourceread.ReadValidatingWebhookConfigurationV1OrDie(bindata.MustAsset("assets/kueue-operator/validatingwebhook.yaml"))
	ownerReference := metav1.OwnerReference{
		APIVersion: "operator.openshift.io/v1alpha1",
		Kind:       "Kueue",
		Name:       kueue.Name,
		UID:        kueue.UID,
	}
	required.OwnerReferences = []metav1.OwnerReference{
		ownerReference,
	}
	controller.EnsureOwnerRef(required, ownerReference)

	for i := range required.Webhooks {
		required.Webhooks[i].ClientConfig.Service.Namespace = kueue.Namespace
	}
	return resourceapply.ApplyValidatingWebhookConfigurationImproved(c.ctx, c.kubeClient.AdmissionregistrationV1(), c.eventRecorder, required, resourceapply.NewResourceCache())
}

func (c *TargetConfigReconciler) manageRoleBindings(kueue *kueuev1alpha1.Kueue, assetPath string) (*rbacv1.RoleBinding, bool, error) {
	required := resourceread.ReadRoleBindingV1OrDie(bindata.MustAsset(assetPath))
	ownerReference := metav1.OwnerReference{
		APIVersion: "operator.openshift.io/v1alpha1",
		Kind:       "Kueue",
		Name:       kueue.Name,
		UID:        kueue.UID,
	}
	required.OwnerReferences = []metav1.OwnerReference{
		ownerReference,
	}
	controller.EnsureOwnerRef(required, ownerReference)

	required.Namespace = kueue.Namespace
	for i := range required.Subjects {
		if required.Subjects[i].Kind != "ServiceAccount" {
			continue
		}
		required.Subjects[i].Namespace = kueue.Namespace
	}
	return resourceapply.ApplyRoleBinding(c.ctx, c.kubeClient.RbacV1(), c.eventRecorder, required)
}

func (c *TargetConfigReconciler) manageClusterRoleBindings(kueue *kueuev1alpha1.Kueue, assetDir string) (*rbacv1.ClusterRoleBinding, bool, error) {
	required := resourceread.ReadClusterRoleBindingV1OrDie(bindata.MustAsset(assetDir))
	ownerReference := metav1.OwnerReference{
		APIVersion: "operator.openshift.io/v1alpha1",
		Kind:       "Kueue",
		Name:       kueue.Name,
		UID:        kueue.UID,
	}
	required.OwnerReferences = []metav1.OwnerReference{
		ownerReference,
	}
	controller.EnsureOwnerRef(required, ownerReference)

	required.Namespace = kueue.Namespace
	for i := range required.Subjects {
		required.Subjects[i].Namespace = kueue.Namespace
	}
	return resourceapply.ApplyClusterRoleBinding(c.ctx, c.kubeClient.RbacV1(), c.eventRecorder, required)
}

func (c *TargetConfigReconciler) manageRole(kueue *kueuev1alpha1.Kueue, assetPath string) (*rbacv1.Role, bool, error) {
	required := resourceread.ReadRoleV1OrDie(bindata.MustAsset(assetPath))
	ownerReference := metav1.OwnerReference{
		APIVersion: "operator.openshift.io/v1alpha1",
		Kind:       "Kueue",
		Name:       kueue.Name,
		UID:        kueue.UID,
	}
	required.OwnerReferences = []metav1.OwnerReference{
		ownerReference,
	}
	controller.EnsureOwnerRef(required, ownerReference)

	required.Namespace = kueue.Namespace
	return resourceapply.ApplyRole(c.ctx, c.kubeClient.RbacV1(), c.eventRecorder, required)
}

func (c *TargetConfigReconciler) manageService(kueue *kueuev1alpha1.Kueue, assetPath string) (*v1.Service, bool, error) {
	required := resourceread.ReadServiceV1OrDie(bindata.MustAsset(assetPath))
	ownerReference := metav1.OwnerReference{
		APIVersion: "operator.openshift.io/v1alpha1",
		Kind:       "Kueue",
		Name:       kueue.Name,
		UID:        kueue.UID,
	}
	required.OwnerReferences = []metav1.OwnerReference{
		ownerReference,
	}
	controller.EnsureOwnerRef(required, ownerReference)

	required.Namespace = kueue.Namespace
	return resourceapply.ApplyService(c.ctx, c.kubeClient.CoreV1(), c.eventRecorder, required)
}

func (c *TargetConfigReconciler) manageClusterRoles(kueue *kueuev1alpha1.Kueue) (map[string]string, error) {
	returnMap := make(map[string]string, 34)
	// This is hardcoded due to the amount of clusterroles that kueue has.
	for i := 0; i < 35; i++ {
		assetPath := fmt.Sprintf("assets/kueue-operator/clusterrole_%d.yml", i)
		clusterRoleName := fmt.Sprintf("clusterrole/clusterrole_%d.yml", i)
		required := resourceread.ReadClusterRoleV1OrDie(bindata.MustAsset(assetPath))
		if required.AggregationRule != nil {
			continue
		}
		ownerReference := metav1.OwnerReference{
			APIVersion: "operator.openshift.io/v1alpha1",
			Kind:       "Kueue",
			Name:       kueue.Name,
			UID:        kueue.UID,
		}
		required.OwnerReferences = []metav1.OwnerReference{
			ownerReference,
		}
		controller.EnsureOwnerRef(required, ownerReference)

		clusterRole, _, err := resourceapply.ApplyClusterRole(c.ctx, c.kubeClient.RbacV1(), c.eventRecorder, required)
		if err != nil {
			return nil, err
		}

		resourceVersion := "0"
		if clusterRole != nil { // SyncConfigMap can return nil
			resourceVersion = clusterRole.ObjectMeta.ResourceVersion
		}
		returnMap[clusterRoleName] = resourceVersion
	}
	return returnMap, nil
}

func (c *TargetConfigReconciler) manageCustomResources(kueue *kueuev1alpha1.Kueue) (map[string]string, error) {
	returnMap := make(map[string]string, 11)
	// This is hardcoded due to the amount of custom resources that kueue has.
	for i := 0; i < 11; i++ {
		assetPath := fmt.Sprintf("assets/kueue-operator/crd_%d.yml", i)
		crdName := fmt.Sprintf("crd/crd_%d.yml", i)
		required := resourceread.ReadCustomResourceDefinitionV1OrDie(bindata.MustAsset(assetPath))
		ownerReference := metav1.OwnerReference{
			APIVersion: "operator.openshift.io/v1alpha1",
			Kind:       "Kueue",
			Name:       kueue.Name,
			UID:        kueue.UID,
		}
		required.OwnerReferences = []metav1.OwnerReference{
			ownerReference,
		}
		controller.EnsureOwnerRef(required, ownerReference)

		crd, _, err := resourceapply.ApplyCustomResourceDefinitionV1(c.ctx, c.crdClient, c.eventRecorder, required)
		if err != nil {
			return nil, err
		}

		resourceVersion := "0"
		if crd != nil { // SyncConfigMap can return nil
			resourceVersion = crd.ObjectMeta.ResourceVersion
		}
		returnMap[crdName] = resourceVersion
	}
	return returnMap, nil
}

func (c *TargetConfigReconciler) manageDeployment(kueueoperator *kueuev1alpha1.Kueue, specAnnotations map[string]string) (*appsv1.Deployment, bool, error) {
	required := resourceread.ReadDeploymentV1OrDie(bindata.MustAsset("assets/kueue-operator/deployment.yaml"))
	required.Name = operatorclient.OperandName
	required.Namespace = kueueoperator.Namespace
	ownerReference := metav1.OwnerReference{
		APIVersion: "operator.openshift.io/v1alpha1",
		Kind:       "Kueue",
		Name:       kueueoperator.Name,
		UID:        kueueoperator.UID,
	}
	required.OwnerReferences = []metav1.OwnerReference{
		ownerReference,
	}
	controller.EnsureOwnerRef(required, ownerReference)

	required.Spec.Template.Spec.Containers[0].Image = kueueoperator.Spec.Image
	switch kueueoperator.Spec.LogLevel {
	case operatorv1.Normal:
		required.Spec.Template.Spec.Containers[0].Args = append(required.Spec.Template.Spec.Containers[0].Args, fmt.Sprintf("-v=%d", 2))
	case operatorv1.Debug:
		required.Spec.Template.Spec.Containers[0].Args = append(required.Spec.Template.Spec.Containers[0].Args, fmt.Sprintf("-v=%d", 4))
	case operatorv1.Trace:
		required.Spec.Template.Spec.Containers[0].Args = append(required.Spec.Template.Spec.Containers[0].Args, fmt.Sprintf("-v=%d", 6))
	case operatorv1.TraceAll:
		required.Spec.Template.Spec.Containers[0].Args = append(required.Spec.Template.Spec.Containers[0].Args, fmt.Sprintf("-v=%d", 8))
	default:
		required.Spec.Template.Spec.Containers[0].Args = append(required.Spec.Template.Spec.Containers[0].Args, fmt.Sprintf("-v=%d", 2))
	}

	resourcemerge.MergeMap(ptr.To(false), &required.Spec.Template.Annotations, specAnnotations)

	deploy, flag, err := resourceapply.ApplyDeployment(
		c.ctx,
		c.kubeClient.AppsV1(),
		c.eventRecorder,
		required,
		resourcemerge.ExpectedDeploymentGeneration(required, kueueoperator.Status.Generations))
	if err != nil {
		klog.InfoS("Deployment error", "Deployment", deploy)
	}
	return deploy, flag, err
}

// Run starts the kube-scheduler and blocks until stopCh is closed.
func (c *TargetConfigReconciler) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Infof("Starting TargetConfigReconciler")
	defer klog.Infof("Shutting down TargetConfigReconciler")

	// doesn't matter what workers say, only start one.
	go wait.Until(c.runWorker, time.Second, stopCh)

	<-stopCh
}

func (c *TargetConfigReconciler) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *TargetConfigReconciler) processNextWorkItem() bool {
	dsKey, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(dsKey)
	item := dsKey.(queueItem)
	err := c.sync(item)
	if err == nil {
		c.queue.Forget(dsKey)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("%v failed with : %v", dsKey, err))
	c.queue.AddRateLimited(dsKey)

	return true
}

// eventHandler queues the operator to check spec and status
func (c *TargetConfigReconciler) eventHandler(item queueItem) cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { c.queue.Add(item) },
		UpdateFunc: func(old, new interface{}) { c.queue.Add(item) },
		DeleteFunc: func(obj interface{}) { c.queue.Add(item) },
	}
}
