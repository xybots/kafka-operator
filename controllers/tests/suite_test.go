// Copyright © 2020 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*

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

package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	ginkoconfig "github.com/onsi/ginkgo/config"

	apiv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	cmv1alpha2 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha2"
	cmv1alpha3 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha3"

	istioclientv1alpha3 "github.com/banzaicloud/istio-client-go/pkg/networking/v1alpha3"
	banzaiistiov1beta1 "github.com/banzaicloud/istio-operator/pkg/apis/istio/v1beta1"

	banzaicloudv1alpha1 "github.com/banzaicloud/kafka-operator/api/v1alpha1"
	banzaicloudv1beta1 "github.com/banzaicloud/kafka-operator/api/v1beta1"
	"github.com/banzaicloud/kafka-operator/controllers"
	"github.com/banzaicloud/kafka-operator/pkg/jmxextractor"
	"github.com/banzaicloud/kafka-operator/pkg/kafkaclient"
	"github.com/banzaicloud/kafka-operator/pkg/scale"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var mockKafkaClients map[types.NamespacedName]kafkaclient.KafkaClient

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	ginkoconfig.DefaultReporterConfig.SlowSpecThreshold = 120

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "base", "crds"),
			// "https://github.com/jetstack/cert-manager/releases/download/v1.1.0/cert-manager.yaml",
			filepath.Join("..", "..", "config", "test", "crd", "cert-manager"),
			filepath.Join("..", "..", "config", "test", "crd", "istio"),
		},
		AttachControlPlaneOutput: false,
	}

	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		err := os.Setenv("KUBEBUILDER_ASSETS", filepath.Join("..", "..", "bin", "kubebuilder", "bin"))
		Expect(err).ToNot(HaveOccurred())
	}

	var err error

	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	scheme := runtime.NewScheme()

	Expect(k8sscheme.AddToScheme(scheme)).To(Succeed())
	Expect(apiextensionsv1beta1.AddToScheme(scheme)).To(Succeed())
	Expect(apiv1.AddToScheme(scheme)).To(Succeed())
	Expect(cmv1alpha2.AddToScheme(scheme)).To(Succeed())
	Expect(cmv1alpha3.AddToScheme(scheme)).To(Succeed())
	Expect(banzaicloudv1alpha1.AddToScheme(scheme)).To(Succeed())
	Expect(banzaicloudv1beta1.AddToScheme(scheme)).To(Succeed())
	Expect(banzaiistiov1beta1.AddToScheme(scheme)).To(Succeed())
	Expect(istioclientv1alpha3.AddToScheme(scheme)).To(Succeed())

	// +kubebuilder:scaffold:scheme

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: "0",
		LeaderElection:     false,
		Port:               8443,
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(mgr).ToNot(BeNil())

	scale.MockNewCruiseControlScaler()
	jmxextractor.NewMockJMXExtractor()

	mockKafkaClients = make(map[types.NamespacedName]kafkaclient.KafkaClient)

	// mock the creation of Kafka clients
	controllers.SetNewKafkaFromCluster(
		func(k8sclient client.Client, cluster *banzaicloudv1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
			client, close := getMockedKafkaClientForCluster(cluster)
			return client, close, nil
		})

	kafkaClusterReconciler := controllers.KafkaClusterReconciler{
		Client:              mgr.GetClient(),
		DirectClient:        mgr.GetAPIReader(),
		Log:                 ctrl.Log.WithName("controllers").WithName("KafkaCluster"),
		Scheme:              mgr.GetScheme(),
		KafkaClientProvider: kafkaclient.NewMockProvider(),
	}

	err = controllers.SetupKafkaClusterWithManager(mgr, kafkaClusterReconciler.Log).Complete(&kafkaClusterReconciler)
	Expect(err).NotTo(HaveOccurred())

	err = controllers.SetupKafkaTopicWithManager(mgr, 10)
	Expect(err).NotTo(HaveOccurred())

	err = controllers.SetupKafkaUserWithManager(mgr, true)
	Expect(err).NotTo(HaveOccurred())

	kafkaClusterCCReconciler := controllers.CruiseControlTaskReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Log:    ctrl.Log.WithName("controller").WithName("CruiseControlTask"),
	}

	err = controllers.SetupCruiseControlWithManager(mgr).Complete(&kafkaClusterCCReconciler)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:builder

	go func() {
		ctrl.Log.Info("starting manager")
		err = mgr.Start(ctrl.SetupSignalHandler())
		Expect(err).ToNot(HaveOccurred())
	}()

	k8sClient, err = client.New(cfg, client.Options{Scheme: mgr.GetScheme()})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	crd := &apiextensionsv1beta1.CustomResourceDefinition{}

	err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: "kafkaclusters.kafka.banzaicloud.io"}, crd)
	Expect(err).NotTo(HaveOccurred())
	Expect(crd.Spec.Names.Kind).To(Equal("KafkaCluster"))

	err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: "kafkatopics.kafka.banzaicloud.io"}, crd)
	Expect(err).NotTo(HaveOccurred())
	Expect(crd.Spec.Names.Kind).To(Equal("KafkaTopic"))

	err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: "kafkausers.kafka.banzaicloud.io"}, crd)
	Expect(err).NotTo(HaveOccurred())
	Expect(crd.Spec.Names.Kind).To(Equal("KafkaUser"))

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
