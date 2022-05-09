package awsplacementgroup

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/machine-api-provider-aws/pkg/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"

	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var (
	cfg        *rest.Config
	testEnv    *envtest.Environment
	testScheme *runtime.Scheme
	k8sClient  client.Client
	ctx        = context.Background()
)

func TestReconciler(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func() {

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "pkg", "api", "machine", "v1"),
			filepath.Join("..", "..", "..", "vendor", "github.com", "openshift", "api", "machine", "v1beta1"),
			filepath.Join("..", "..", "..", "vendor", "github.com", "openshift", "api", "config", "v1"),
		},
		ErrorIfCRDPathMissing: true,
	}

	testScheme = scheme.Scheme
	Expect(machinev1.Install(testScheme)).To(Succeed())
	Expect(machinev1beta1.Install(testScheme)).To(Succeed())
	Expect(configv1.AddToScheme(testScheme)).To(Succeed())

	// Debug logging, uncomment and test with `-v` to inspect them.
	// log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	// Set a higher than default Eventually timeout to account for some API hiccups.
	SetDefaultEventuallyTimeout(10 * time.Second)

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	komega.SetClient(k8sClient)
	komega.SetContext(ctx)
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
