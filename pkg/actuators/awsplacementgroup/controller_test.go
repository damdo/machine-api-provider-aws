package awsplacementgroup

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/machine-api-provider-aws/pkg/api/machine/v1"

	awsclient "github.com/openshift/machine-api-provider-aws/pkg/client"
	resourcebuilder "github.com/openshift/machine-api-provider-aws/pkg/resourcebuilder"

	"github.com/golang/mock/gomock"
	mockaws "github.com/openshift/machine-api-provider-aws/pkg/client/mock"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	infrastructureName string = "cluster"
	pgName             string = "pg-test"
	pgID               string = "pg-testid"
)

type placementOptions struct {
	strategy string
	count    int64
}

func baselineAWSExpects(mocker *mockaws.MockClient, options placementOptions) {

	mocker.EXPECT().
		DescribePlacementGroups(&ec2.DescribePlacementGroupsInput{
			GroupNames: []*string{aws.String(pgName)},
		}).
		Return(&ec2.DescribePlacementGroupsOutput{
			PlacementGroups: []*ec2.PlacementGroup{},
		}, nil).
		Times(1)

	mocker.EXPECT().
		CreatePlacementGroup(gomock.Any()).
		Return(&ec2.CreatePlacementGroupOutput{
			PlacementGroup: &ec2.PlacementGroup{
				GroupId:        aws.String(pgID),
				GroupName:      aws.String(pgName),
				Strategy:       aws.String(strings.ToLower(options.strategy)),
				PartitionCount: aws.Int64(options.count),
			}}, nil).
		AnyTimes()

	mocker.EXPECT().
		DescribePlacementGroups(&ec2.DescribePlacementGroupsInput{
			GroupNames: []*string{aws.String(pgName)}}).
		Return(&ec2.DescribePlacementGroupsOutput{
			PlacementGroups: []*ec2.PlacementGroup{
				{
					// GroupArn:  aws.String("arn"),
					GroupId:        aws.String(pgID),
					GroupName:      aws.String(pgName),
					State:          aws.String("up"),
					Strategy:       aws.String(strings.ToLower(options.strategy)),
					PartitionCount: aws.Int64(options.count),
					Tags:           nil,
				},
			},
		}, nil).
		AnyTimes()
}

var _ = Describe("AWSPlacementGroupReconciler", func() {
	var namespaceName string
	var mgrCancel context.CancelFunc
	var mgrDone chan struct{}
	var fakeRecorder *record.FakeRecorder
	var mockCtrl *gomock.Controller
	var mockAWSClient *mockaws.MockClient

	BeforeEach(func() {
		By("Setting up a namespace for the test")
		namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "pg-test-"}}
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
		namespaceName = namespace.GetName()

		By("Setting up a new infrastructure for the test")
		// Create infrastructure object.
		infra := resourcebuilder.Infrastructure().WithName(infrastructureName).AsAWS("test", "eu-west-2").Build()
		infraStatus := infra.Status.DeepCopy()
		Expect(k8sClient.Create(ctx, infra)).To(Succeed())
		// Update Infrastructure Status.
		Eventually(komega.UpdateStatus(infra, func() {
			infra.Status = *infraStatus
		})).Should(Succeed())

		By("Setting up a manager and controller")
		mgr, err := manager.New(cfg, manager.Options{
			MetricsBindAddress: "0",
			Namespace:          namespaceName,
		})
		Expect(err).ToNot(HaveOccurred(), "Manager should be able to be created")
		// Set up a mock AWS Client.
		mockCtrl = gomock.NewController(GinkgoT())
		mockAWSClient = mockaws.NewMockClient(mockCtrl)
		awsClientBuilder := func(client client.Client, secretName, namespace, region string, configManagedClient client.Client) (awsclient.Client, error) {
			return mockAWSClient, nil
		}
		// Set up the reconciler.
		reconciler := &Reconciler{
			Client:              mgr.GetClient(),
			Log:                 log.Log,
			ConfigManagedClient: mgr.GetClient(),
			AWSClientBuilder:    awsClientBuilder,
		}
		fakeRecorder = record.NewFakeRecorder(1)
		reconciler.recorder = fakeRecorder
		// Create a controller out of the reconciler and set it up with the manager.
		Expect(reconciler.SetupWithManager(mgr, controller.Options{})).To(Succeed(), "Reconciler should be able to setup with manager")

		By("Starting the manager")
		var mgrCtx context.Context
		mgrCtx, mgrCancel = context.WithCancel(ctx)
		mgrDone = make(chan struct{})

		go func() {
			defer GinkgoRecover()
			defer close(mgrDone)

			Expect(mgr.Start(mgrCtx)).To(Succeed())
		}()

	})

	AfterEach(func() {
		By("Stopping the manager")
		mgrCancel()
		// Wait for the mgrDone to be closed, which will happen once the mgr has stopped
		<-mgrDone
		Expect(deleteAWSPlacementGroups(k8sClient, namespaceName)).To(Succeed())
		Expect(deleteInfrastructure(k8sClient, infrastructureName)).To(Succeed())
	})

	Context("when Creating a AWSPlacementGroup", func() {

		Context("with Managed ManagementState", func() {
			var pg *machinev1.AWSPlacementGroup
			JustBeforeEach(func() {
				baselineAWSExpects(mockAWSClient, placementOptions{strategy: string(machinev1.AWSClusterPlacementGroupType)})
				pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().AsClusterType().Build()
				Expect(k8sClient.Create(ctx, pg)).Should(Succeed())
			})

			It("should have the Status.ManagementState updated to Unmanaged", func() {
				Eventually(komega.Object(pg)).Should(HaveField("Status.ManagementState", Equal(machinev1.ManagedManagementState)))
			})

			It("should add the awsplacementgroup.machine.openshift.io finalizer", func() {
				Eventually(komega.Object(pg)).Should(HaveField("ObjectMeta.Finalizers", ContainElement(awsPlacementGroupFinalizer)))
			})

			It("should populate the .Status.ObservedConfiguration", func() {
				Eventually(komega.Object(pg)).Should(HaveField("Status.ObservedConfiguration", Not(BeNil())))
			})

			It("should populate the .Status.ExpiresAt", func() {
				Eventually(komega.Object(pg)).Should(HaveField("Status.ExpiresAt", Not(BeNil())))
			})
		})

		Context("with Unmanaged ManagementState", func() {
			var pg *machinev1.AWSPlacementGroup
			JustBeforeEach(func() {
				mockAWSClient.EXPECT().
					DescribePlacementGroups(&ec2.DescribePlacementGroupsInput{
						GroupNames: []*string{aws.String(pgName)},
					}).
					Return(&ec2.DescribePlacementGroupsOutput{
						PlacementGroups: []*ec2.PlacementGroup{},
					}, nil).
					Times(2)

				pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsUnmanaged().Build()
				Expect(k8sClient.Create(ctx, pg)).Should(Succeed())

				// Wait for the status to not be empty, to ensure that at least one reconcile happens.
				Eventually(komega.Object(pg)).Should(HaveField("Status.ManagementState", Equal(machinev1.UnmanagedManagementState)))
			})

			It("should not add the awsplacementgroup.machine.openshift.io finalizer", func() {
				Eventually(komega.Object(pg)).ShouldNot(HaveField("ObjectMeta.Finalizers", ContainElement(awsPlacementGroupFinalizer)))
			})

			// Even Unmanaged AWSPlacementGroups should have their ObservedConfig populated.
			It("should populate the .Status.ObservedConfiguration", func() {
				Eventually(komega.Object(pg)).Should(HaveField("Status.ObservedConfiguration", Not(BeNil())))
			})

			// Even Unmanaged AWSPlacementGroups should have their ExpiresAt populated.
			It("should populate the .Status.ExpiresAt", func() {
				Eventually(komega.Object(pg)).Should(HaveField("Status.ExpiresAt", Not(BeNil())))
			})
		})

		Context("with Managed ManagementState but empty Managed field", func() {
			var pg *machinev1.AWSPlacementGroup
			JustBeforeEach(func() {
				baselineAWSExpects(mockAWSClient, placementOptions{strategy: string(machinev1.AWSClusterPlacementGroupType)})
				pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().Build()
				Expect(k8sClient.Create(ctx, pg)).Should(Succeed())
			})

			It("should have empty Status.ManagementState", func() {
				Eventually(komega.Object(pg)).Should(HaveField("Status.ManagementState", BeEquivalentTo("")))
			})

		})

	})

	Context("when Updating an existing AWSPlacementGroup", func() {

		Context("with Managed ManagementState to Unmanaged ManagementState", func() {
			var pg *machinev1.AWSPlacementGroup
			JustBeforeEach(func() {
				baselineAWSExpects(mockAWSClient, placementOptions{strategy: string(machinev1.AWSClusterPlacementGroupType)})

				// Create the AWSPlacementGroup.
				pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().AsClusterType().Build()
				Expect(k8sClient.Create(ctx, pg)).Should(Succeed())

				// Wait for the status to not be empty, to ensure that at least one reconcile happens.
				Eventually(komega.Object(pg)).Should(HaveField("Status.ManagementState", Equal(machinev1.ManagedManagementState)))

				// Check the finalizer is there first.
				Eventually(komega.Object(pg)).Should(HaveField("ObjectMeta.Finalizers", ContainElement(awsPlacementGroupFinalizer)))

				// Update the ManagementState to Unmanaged.
				Eventually(func() error {
					pgUnmanaged := &machinev1.AWSPlacementGroup{}
					if err := k8sClient.Get(ctx, types.NamespacedName{Name: pgName, Namespace: namespaceName}, pgUnmanaged); err != nil {
						return err
					}
					pgUnmanaged.Spec.ManagementSpec.ManagementState = machinev1.UnmanagedManagementState
					return k8sClient.Update(ctx, pgUnmanaged)
				}).Should(Succeed())
			})

			It("should have the Status.ManagementState updated to Unmanaged", func() {
				Eventually(komega.Object(pg)).Should(HaveField("Status.ManagementState", Equal(machinev1.UnmanagedManagementState)))
			})

			It("should have awsplacementgroup.machine.openshift.io finalizer", func() {
				Eventually(komega.Object(pg)).ShouldNot(HaveField("ObjectMeta.Finalizers", ContainElement(awsPlacementGroupFinalizer)))
			})
		})

		Context("with Unmanaged ManagementState to Managed ManagementState", func() {
			var pg *machinev1.AWSPlacementGroup
			JustBeforeEach(func() {
				baselineAWSExpects(mockAWSClient, placementOptions{strategy: string(machinev1.AWSClusterPlacementGroupType)})

				// Create a new Unmanaged AWSPlacementGroup.
				pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsUnmanaged().Build()
				Expect(k8sClient.Create(ctx, pg)).Should(Succeed())

				// Wait for the status to not be empty, to ensure that at least one reconcile happens.
				Eventually(komega.Object(pg)).Should(HaveField("Status.ManagementState", Equal(machinev1.UnmanagedManagementState)))

				// Update the ManagementState to Managed.
				Eventually(func() error {
					pgUnmanaged := &machinev1.AWSPlacementGroup{}
					if err := k8sClient.Get(ctx, types.NamespacedName{Name: pgName, Namespace: namespaceName}, pgUnmanaged); err != nil {
						return err
					}
					pgUnmanaged.Spec.ManagementSpec.ManagementState = machinev1.ManagedManagementState
					return k8sClient.Update(ctx, pgUnmanaged)
				}).Should(Succeed())
			})

			It("should maintain Status.ManagementState as Unmanaged", func() {
				Eventually(komega.Object(pg)).Should(HaveField("Status.ManagementState", Equal(machinev1.UnmanagedManagementState)))
			})
		})

	})

	Context("when populating the ObservedConfiguration", func() {
		var pg *machinev1.AWSPlacementGroup

		It("should have the correct .Status.ObservedConfiguration.GroupType", func() {
			baselineAWSExpects(mockAWSClient, placementOptions{strategy: string(machinev1.AWSClusterPlacementGroupType)})
			// Create a Managed AWSPlacementGroup of type Cluster.
			pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().AsClusterType().Build()
			Expect(k8sClient.Create(ctx, pg)).Should(Succeed())
			Eventually(komega.Object(pg)).Should(HaveField("Status.ObservedConfiguration.GroupType", Equal(machinev1.AWSClusterPlacementGroupType)))
		})

		It("should have the correct .Status.ObservedConfiguration.Partition when GroupType is not Partition", func() {
			baselineAWSExpects(mockAWSClient, placementOptions{strategy: string(machinev1.AWSClusterPlacementGroupType)})
			pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().AsClusterType().Build()
			Expect(k8sClient.Create(ctx, pg)).Should(Succeed())
			Eventually(komega.Object(pg)).Should(HaveField("Status.ObservedConfiguration.Partition", BeNil()))
		})

		It("should have the correct .Status.ObservedConfiguration.Partition when GroupType is Partition", func() {
			var count int64 = 1
			baselineAWSExpects(mockAWSClient, placementOptions{strategy: string(machinev1.AWSPartitionPlacementGroupType), count: int64(count)})
			pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().AsPartitionTypeWithCount(int32(count)).Build()
			Expect(k8sClient.Create(ctx, pg)).Should(Succeed())
			Eventually(komega.Object(pg)).Should(HaveField("Status.ObservedConfiguration.Partition", Not(BeNil())))
			Eventually(komega.Object(pg)).Should(HaveField("Status.ObservedConfiguration.Partition.Count", BeEquivalentTo(1)))
		})
	})
})

var _ = Describe("AWSPlacementGroupValidation", func() {
	namespaceName := "ns-test"

	Context("when ManagementState is Managed and the Managed field is empty", func() {
		var pg *machinev1.AWSPlacementGroup
		var validateErr error

		JustBeforeEach(func() {
			pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().Build()
			By("Validating the AWSPlacementGroup")
			validateErr = validateAWSPlacementGroup(pg)
		})

		It("should error with invalid error", func() {
			Expect(validateErr).To(MatchError(fmt.Sprintf("invalid aws placement group. spec.managementSpec.managed "+
				"must not be nil when spec.managementSpec.managementState is %s", machinev1.ManagedManagementState)))
		})
	})

	Context("when ManagementState is empty", func() {
		var pg *machinev1.AWSPlacementGroup
		var validateErr error

		JustBeforeEach(func() {
			// Define a Cluster type AWSPlacementGroup.
			pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().AsClusterType().Build()
			// Then alter its ManagementState to be empty.
			pg.Spec.ManagementSpec.ManagementState = ""
			By("Validating the AWSPlacementGroup")
			validateErr = validateAWSPlacementGroup(pg)
		})

		It("should error with invalid error", func() {
			Expect(validateErr).To(MatchError(fmt.Sprintf("invalid aws placement group. spec.managementSpec.managementState must either be %s or %s",
				machinev1.ManagedManagementState, machinev1.UnmanagedManagementState)))
		})
	})

	Context("when ManagementState has a non valid value", func() {
		var pg *machinev1.AWSPlacementGroup
		var validateErr error

		JustBeforeEach(func() {
			// Define a Cluster type AWSPlacementGroup.
			pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().AsClusterType().Build()
			// Then alter its ManagementState to be a non valid value.
			pg.Spec.ManagementSpec.ManagementState = "Inv$alid"

			By("Validating the AWSPlacementGroup")
			validateErr = validateAWSPlacementGroup(pg)
		})

		It("should error with invalid error", func() {
			Expect(validateErr).To(MatchError(fmt.Sprintf("invalid aws placement group. spec.managementSpec.managementState must either be %s or %s",
				machinev1.ManagedManagementState, machinev1.UnmanagedManagementState)))
		})
	})

	Context("when ManagementState has Managed value", func() {
		var pg *machinev1.AWSPlacementGroup
		var validateErr error

		JustBeforeEach(func() {
			pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().AsClusterType().Build()
			By("Validating the AWSPlacementGroup")
			validateErr = validateAWSPlacementGroup(pg)
		})

		It("should not error", func() {
			Expect(validateErr).To(BeNil())
		})
	})

	Context("when ManagementState has Unmanaged value", func() {
		var pg *machinev1.AWSPlacementGroup
		var validateErr error

		JustBeforeEach(func() {
			pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsUnmanaged().Build()
			By("Validating the AWSPlacementGroup")
			validateErr = validateAWSPlacementGroup(pg)
		})

		It("should not error", func() {
			Expect(validateErr).To(BeNil())
		})
	})

	Context("when Managed GroupType is empty", func() {
		var pg *machinev1.AWSPlacementGroup
		var validateErr error

		JustBeforeEach(func() {
			// Define a Cluster type AWSPlacementGroup.
			pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().AsClusterType().Build()
			// Then alter its GroupType to be empty.
			pg.Spec.ManagementSpec.Managed.GroupType = ""
			By("Validating the AWSPlacementGroup")
			validateErr = validateAWSPlacementGroup(pg)
		})

		It("should error with invalid error", func() {
			Expect(validateErr).To(MatchError(fmt.Sprintf("invalid aws placement group. spec.managementSpec.managed.groupType must either be %s, %s or %s",
				machinev1.AWSClusterPlacementGroupType, machinev1.AWSPartitionPlacementGroupType, machinev1.AWSSpreadPlacementGroupType)))
		})
	})

	Context("when Managed GroupType has a non valid value", func() {
		var pg *machinev1.AWSPlacementGroup
		var validateErr error

		JustBeforeEach(func() {
			// Define a Cluster type AWSPlacementGroup.
			pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().AsClusterType().Build()
			// Then alter its GroupType to be empty.
			pg.Spec.ManagementSpec.Managed.GroupType = "Inv$alid"
			By("Validating the AWSPlacementGroup")
			validateErr = validateAWSPlacementGroup(pg)
		})

		It("should error with invalid error", func() {
			Expect(validateErr).To(MatchError(fmt.Sprintf("invalid aws placement group. spec.managementSpec.managed.groupType must either be %s, %s or %s",
				machinev1.AWSClusterPlacementGroupType, machinev1.AWSPartitionPlacementGroupType, machinev1.AWSSpreadPlacementGroupType)))
		})
	})

	Context("when Managed GroupType has Cluster value", func() {
		var pg *machinev1.AWSPlacementGroup
		var validateErr error

		JustBeforeEach(func() {
			pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().AsClusterType().Build()
			By("Validating the AWSPlacementGroup")
			validateErr = validateAWSPlacementGroup(pg)
		})

		It("should not error", func() {
			Expect(validateErr).To(BeNil())
		})
	})

	Context("when Managed GroupType has Spread value", func() {
		var pg *machinev1.AWSPlacementGroup
		var validateErr error

		JustBeforeEach(func() {
			pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().AsSpreadType().Build()
			By("Validating the AWSPlacementGroup")
			validateErr = validateAWSPlacementGroup(pg)
		})

		It("should not error", func() {
			Expect(validateErr).To(BeNil())
		})
	})

	Context("when Managed GroupType has Partition value", func() {
		var pg *machinev1.AWSPlacementGroup
		var validateErr error

		JustBeforeEach(func() {
			pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().AsPartitionType().Build()
			By("Validating the AWSPlacementGroup")
			validateErr = validateAWSPlacementGroup(pg)
		})

		It("should not error", func() {
			Expect(validateErr).To(BeNil())
		})
	})

	Context("when Managed GroupType has Partition and a valid Partition.Count", func() {
		var pg *machinev1.AWSPlacementGroup
		var validateErr error

		JustBeforeEach(func() {
			pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().AsPartitionTypeWithCount(3).Build()
			By("Validating the AWSPlacementGroup")
			validateErr = validateAWSPlacementGroup(pg)
		})

		It("should not error", func() {
			Expect(validateErr).To(BeNil())
		})
	})

	Context("when Managed GroupType has Partition and a too low Partition.Count", func() {
		var pg *machinev1.AWSPlacementGroup
		var validateErr error

		JustBeforeEach(func() {
			pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().AsPartitionTypeWithCount(0).Build()
			By("Validating the AWSPlacementGroup")
			validateErr = validateAWSPlacementGroup(pg)
		})

		It("should error with invalid error", func() {
			Expect(validateErr).To(MatchError("invalid aws placement group. spec.managementSpec.managed.partition.count " +
				"must be greater or equal than 1 and less or equal than 7"))
		})
	})

	Context("when Managed GroupType has Partition and a too high Partition.Count", func() {
		var pg *machinev1.AWSPlacementGroup
		var validateErr error

		JustBeforeEach(func() {
			pg = resourcebuilder.AWSPlacementGroup().WithName(pgName).WithNamespace(namespaceName).AsManaged().AsPartitionTypeWithCount(8).Build()
			By("Validating the AWSPlacementGroup")
			validateErr = validateAWSPlacementGroup(pg)
		})

		It("should error with invalid error", func() {
			Expect(validateErr).To(MatchError("invalid aws placement group. spec.managementSpec.managed.partition.count " +
				"must be greater or equal than 1 and less or equal than 7"))
		})
	})

})

// removeFinalizers removes all finalizers from the object given.
// Finalizers must be removed one by one else the API server will reject the update.
func removeFinalizers(obj client.Object) {
	filter := func(finalizers []string, toRemove string) []string {
		out := []string{}

		for _, f := range finalizers {
			if f != toRemove {
				out = append(out, f)
			}
		}

		return out
	}

	for _, finalizer := range obj.GetFinalizers() {
		Eventually(komega.Update(obj, func() {
			obj.SetFinalizers(filter(obj.GetFinalizers(), finalizer))
		})).Should(Succeed())
	}
}

func deleteAWSPlacementGroups(c client.Client, namespaceName string) error {
	placementGroups := &machinev1.AWSPlacementGroupList{}
	err := c.List(ctx, placementGroups, client.InNamespace(namespaceName))
	if err != nil {
		return err
	}

	for _, pg := range placementGroups.Items {
		removeFinalizers(&pg)
		err := c.Delete(ctx, &pg)
		if err != nil {
			return err
		}
	}

	Eventually(func() error {
		placementGroups := &machinev1.AWSPlacementGroupList{}
		err := c.List(ctx, placementGroups)
		if err != nil {
			return err
		}
		if len(placementGroups.Items) > 0 {
			return fmt.Errorf("AWSPlacementGroups not deleted")
		}
		return nil
	}).Should(Succeed())

	return nil
}

func deleteInfrastructure(c client.Client, insfrastructureName string) error {
	// TODO(dam): do this properly
	infra := &configv1.Infrastructure{}
	err := c.Get(ctx, client.ObjectKey{Name: insfrastructureName}, infra)
	if err != nil {
		return err
	}

	if err := c.Delete(ctx, infra); err != nil {
		return err
	}

	Eventually(func() error {
		infras := &configv1.InfrastructureList{}
		err := c.List(ctx, infras)
		if err != nil {
			return err
		}
		if len(infras.Items) > 0 {
			return fmt.Errorf("Infrastructures not deleted")
		}
		return nil
	}).Should(Succeed())

	return nil
}
