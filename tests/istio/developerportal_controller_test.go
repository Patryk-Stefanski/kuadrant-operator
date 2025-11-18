//go:build integration

package istio_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kuadrantv1beta1 "github.com/kuadrant/kuadrant-operator/api/v1beta1"
	"github.com/kuadrant/kuadrant-operator/internal/kuadrant"
)

var _ = Describe("Developer Portal Controller", Serial, func() {
	const (
		testTimeOut      = SpecTimeout(2 * time.Minute)
		afterEachTimeOut = NodeTimeout(3 * time.Minute)
	)

	getKuadrantCR := func(ctx context.Context, cl client.Client) *kuadrantv1beta1.Kuadrant {
		kuadrantList := &kuadrantv1beta1.KuadrantList{}
		err := cl.List(ctx, kuadrantList)
		// must exist
		Expect(err).ToNot(HaveOccurred())
		Expect(kuadrantList.Items).To(HaveLen(1))
		return &kuadrantList.Items[0]
	}

	Context("when developer portal is enabled and then disabled", func() {
		It("creates and deletes all required resources", func(ctx SpecContext) {
			// Resources to check
			clusterRole := &rbacv1.ClusterRole{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ClusterRole",
					APIVersion: rbacv1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "developer-portal-controller-manager-role",
				},
			}

			serviceAccount := &corev1.ServiceAccount{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceAccount",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "developer-portal-controller",
					Namespace: "",
				},
			}

			leaderElectionRole := &rbacv1.Role{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Role",
					APIVersion: rbacv1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "developer-portal-controller-leader-election-role",
					Namespace: "",
				},
			}

			leaderElectionRoleBinding := &rbacv1.RoleBinding{
				TypeMeta: metav1.TypeMeta{
					Kind:       "RoleBinding",
					APIVersion: rbacv1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "developer-portal-controller-leader-election-rolebinding",
					Namespace: "",
				},
			}

			clusterRoleBinding := &rbacv1.ClusterRoleBinding{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ClusterRoleBinding",
					APIVersion: rbacv1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "developer-portal-controller-rolebinding",
				},
			}

			deployment := &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "developer-portal-controller",
					Namespace: "",
				},
			}

			kuadrantCR := getKuadrantCR(ctx, testClient())

			const roleNamespace = "kuadrant-system"
			leaderElectionRole.Namespace = roleNamespace
			leaderElectionRoleBinding.Namespace = roleNamespace
			serviceAccount.Namespace = kuadrantCR.Namespace
			deployment.Namespace = kuadrantCR.Namespace

			// Verify the leader election Role exists in kuadrant-system
			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(leaderElectionRole), leaderElectionRole)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(leaderElectionRole.Labels).To(HaveKeyWithValue(kuadrant.DeveloperPortalLabel, "true"))
			}).WithContext(ctx).Should(Succeed())

			// Verify ClusterRole exists and has correct labels and rules
			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(clusterRole), clusterRole)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(clusterRole.Labels).To(HaveKeyWithValue(kuadrant.DeveloperPortalLabel, "true"))
			}).WithContext(ctx).Should(Succeed())

			// Enable developer portal
			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(kuadrantCR), kuadrantCR)
				g.Expect(err).NotTo(HaveOccurred())
				kuadrantCR.Spec.Components.DeveloperPortal.Enabled = true
				err = testClient().Update(ctx, kuadrantCR)
				g.Expect(err).NotTo(HaveOccurred())
			}).WithTimeout(5 * time.Minute).WithContext(ctx).Should(Succeed())

			// Verify ServiceAccount is created
			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(serviceAccount), serviceAccount)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(serviceAccount.Labels).To(HaveKeyWithValue("app", "developer-portal-controller"))
				g.Expect(serviceAccount.Labels).To(HaveKeyWithValue(kuadrant.DeveloperPortalLabel, "true"))
				g.Expect(serviceAccount.OwnerReferences).To(HaveLen(1))
				g.Expect(serviceAccount.OwnerReferences[0].Kind).To(Equal("Kuadrant"))
				g.Expect(*serviceAccount.OwnerReferences[0].Controller).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

			// Verify leader election RoleBinding is created
			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(leaderElectionRoleBinding), leaderElectionRoleBinding)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(leaderElectionRoleBinding.Labels).To(HaveKeyWithValue("app", "developer-portal-controller"))
				g.Expect(leaderElectionRoleBinding.Labels).To(HaveKeyWithValue(kuadrant.DeveloperPortalLabel, "true"))
				g.Expect(leaderElectionRoleBinding.RoleRef.Name).To(Equal("developer-portal-controller-leader-election-role"))
				g.Expect(leaderElectionRoleBinding.RoleRef.Kind).To(Equal("Role"))
				g.Expect(leaderElectionRoleBinding.Subjects).To(HaveLen(1))
				g.Expect(leaderElectionRoleBinding.Subjects[0].Kind).To(Equal("ServiceAccount"))
				g.Expect(leaderElectionRoleBinding.Subjects[0].Name).To(Equal("developer-portal-controller"))
				g.Expect(leaderElectionRoleBinding.Subjects[0].Namespace).To(Equal(kuadrantCR.Namespace))
			}).WithContext(ctx).Should(Succeed())

			// Verify ClusterRoleBinding is created
			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(clusterRoleBinding), clusterRoleBinding)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(clusterRoleBinding.Labels).To(HaveKeyWithValue("app", "developer-portal-controller"))
				g.Expect(clusterRoleBinding.Labels).To(HaveKeyWithValue(kuadrant.DeveloperPortalLabel, "true"))
				g.Expect(clusterRoleBinding.RoleRef.Name).To(Equal("developer-portal-controller-manager-role"))
				g.Expect(clusterRoleBinding.RoleRef.Kind).To(Equal("ClusterRole"))
				g.Expect(clusterRoleBinding.Subjects).To(HaveLen(1))
				g.Expect(clusterRoleBinding.Subjects[0].Kind).To(Equal("ServiceAccount"))
				g.Expect(clusterRoleBinding.Subjects[0].Name).To(Equal("developer-portal-controller"))
				g.Expect(clusterRoleBinding.Subjects[0].Namespace).To(Equal(kuadrantCR.Namespace))
			}).WithContext(ctx).Should(Succeed())

			// Verify Deployment is created
			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(deployment), deployment)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(deployment.Labels).To(HaveKeyWithValue("app", "developer-portal-controller"))
				g.Expect(deployment.Labels).To(HaveKeyWithValue(kuadrant.DeveloperPortalLabel, "true"))
				g.Expect(deployment.OwnerReferences).To(HaveLen(1))
				g.Expect(deployment.OwnerReferences[0].Kind).To(Equal("Kuadrant"))
				// Verify deployment spec
				g.Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
				g.Expect(deployment.Spec.Template.Spec.ServiceAccountName).To(Equal("developer-portal-controller"))
				// Verify container
				g.Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
				container := deployment.Spec.Template.Spec.Containers[0]
				g.Expect(container.Name).To(Equal("manager"))
				g.Expect(container.Image).To(ContainSubstring("developer-portal-controller"))
				g.Expect(container.Command).To(ContainElement("/manager"))
				g.Expect(container.Args).To(ContainElement("--leader-elect"))
				// Verify probes exist
				g.Expect(container.LivenessProbe).NotTo(BeNil())
				g.Expect(container.LivenessProbe.HTTPGet.Path).To(Equal("/healthz"))
				g.Expect(container.ReadinessProbe).NotTo(BeNil())
				g.Expect(container.ReadinessProbe.HTTPGet.Path).To(Equal("/readyz"))
			}).WithContext(ctx).Should(Succeed())

			// Now disable developer portal
			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(kuadrantCR), kuadrantCR)
				g.Expect(err).NotTo(HaveOccurred())
				kuadrantCR.Spec.Components.DeveloperPortal.Enabled = false
				err = testClient().Update(ctx, kuadrantCR)
				g.Expect(err).NotTo(HaveOccurred())
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(clusterRole), clusterRole)
				g.Expect(err).NotTo(HaveOccurred(), "ClusterRole should still exist")
			}).WithContext(ctx).Should(Succeed())

			// Verify ServiceAccount is deleted
			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(serviceAccount), serviceAccount)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(leaderElectionRole), leaderElectionRole)
				g.Expect(err).NotTo(HaveOccurred(), "Leader election Role should still exist")
			}).WithContext(ctx).Should(Succeed())

			// Verify leader election RoleBinding is deleted
			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(leaderElectionRoleBinding), leaderElectionRoleBinding)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

			// Verify ClusterRoleBinding is deleted
			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(clusterRoleBinding), clusterRoleBinding)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

			// Verify Deployment is deleted
			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(deployment), deployment)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

		}, testTimeOut)
	})

	Context("when Kuadrant CR is deleted without disabling developer portal first", func() {
		var savedKuadrantCR *kuadrantv1beta1.Kuadrant

		BeforeEach(func(ctx SpecContext) {
			// Save the current Kuadrant CR state before the test
			savedKuadrantCR = getKuadrantCR(ctx, testClient()).DeepCopy()
		})

		AfterEach(func(ctx SpecContext) {
			// Recreate the Kuadrant CR after the test deletes it
			if savedKuadrantCR != nil {
				kuadrantCR := &kuadrantv1beta1.Kuadrant{
					ObjectMeta: metav1.ObjectMeta{
						Name:      savedKuadrantCR.Name,
						Namespace: savedKuadrantCR.Namespace,
					},
					Spec: savedKuadrantCR.Spec,
				}

				err := testClient().Create(ctx, kuadrantCR)
				Expect(err).NotTo(HaveOccurred())

				// Wait for the CR to be ready
				Eventually(func(g Gomega) {
					recreatedCR := &kuadrantv1beta1.Kuadrant{}
					err := testClient().Get(ctx, client.ObjectKeyFromObject(kuadrantCR), recreatedCR)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(meta.IsStatusConditionTrue(recreatedCR.Status.Conditions, "Ready")).To(BeTrue())
				}).WithContext(ctx).WithTimeout(3 * time.Minute).Should(Succeed())
			}
		}, afterEachTimeOut)

		It("uses finalizer to ensure cleanup happens before deletion", func(ctx SpecContext) {
			const (
				developerPortalFinalizer = "kuadrant.io/developerportal"
			)

			// Resources to check
			serviceAccount := &corev1.ServiceAccount{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceAccount",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "developer-portal-controller",
					Namespace: "",
				},
			}

			leaderElectionRoleBinding := &rbacv1.RoleBinding{
				TypeMeta: metav1.TypeMeta{
					Kind:       "RoleBinding",
					APIVersion: rbacv1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "developer-portal-controller-leader-election-rolebinding",
					Namespace: "",
				},
			}

			clusterRoleBinding := &rbacv1.ClusterRoleBinding{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ClusterRoleBinding",
					APIVersion: rbacv1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "developer-portal-controller-rolebinding",
				},
			}

			deployment := &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "developer-portal-controller",
					Namespace: "",
				},
			}

			kuadrantCR := getKuadrantCR(ctx, testClient())

			const roleNamespace = "kuadrant-system"
			leaderElectionRoleBinding.Namespace = roleNamespace
			serviceAccount.Namespace = kuadrantCR.Namespace
			deployment.Namespace = kuadrantCR.Namespace

			// Enable developer portal
			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(kuadrantCR), kuadrantCR)
				g.Expect(err).NotTo(HaveOccurred())
				kuadrantCR.Spec.Components.DeveloperPortal.Enabled = true
				err = testClient().Update(ctx, kuadrantCR)
				g.Expect(err).NotTo(HaveOccurred())
			}).WithTimeout(5 * time.Minute).WithContext(ctx).Should(Succeed())

			// Verify ServiceAccount is created
			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(serviceAccount), serviceAccount)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(serviceAccount.Labels).To(HaveKeyWithValue("app", "developer-portal-controller"))
			}).WithContext(ctx).Should(Succeed())

			// Verify Deployment is created
			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(deployment), deployment)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(deployment.Labels).To(HaveKeyWithValue("app", "developer-portal-controller"))
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(kuadrantCR), kuadrantCR)
				g.Expect(err).NotTo(HaveOccurred())
				err = testClient().Delete(ctx, kuadrantCR)
				g.Expect(err).NotTo(HaveOccurred())
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(kuadrantCR), kuadrantCR)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(kuadrantCR.GetDeletionTimestamp()).NotTo(BeNil(), "CR should have deletion timestamp")
				g.Expect(kuadrantCR.GetFinalizers()).To(ContainElement(developerPortalFinalizer), "finalizer should be present during cleanup")
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(serviceAccount), serviceAccount)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(leaderElectionRoleBinding), leaderElectionRoleBinding)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(clusterRoleBinding), clusterRoleBinding)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(deployment), deployment)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega) {
				err := testClient().Get(ctx, client.ObjectKeyFromObject(kuadrantCR), kuadrantCR)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

		}, testTimeOut)
	})
})
