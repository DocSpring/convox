package k8s

import (
	"context"
	"testing"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmfake "github.com/cert-manager/cert-manager/pkg/client/clientset/versioned/fake"
	"github.com/stretchr/testify/require"
	am "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ingressOwnedCertificate(ns, name string) *cmapi.Certificate {
	return &cmapi.Certificate{
		ObjectMeta: am.ObjectMeta{
			Namespace: ns,
			Name:      name,
			OwnerReferences: []am.OwnerReference{
				{APIVersion: "networking.k8s.io/v1", Kind: "Ingress", Name: "web", UID: "ingress-uid"},
				{APIVersion: "example.org/v1", Kind: "Other", Name: "keep", UID: "other-uid"},
			},
		},
		Spec: cmapi.CertificateSpec{SecretName: name},
	}
}

func TestReleaseAdoptIngressCertificates(t *testing.T) {
	for _, tc := range []struct {
		routerType string
		migration  bool
		adopt      bool
	}{
		{"contour", false, true},
		{"contour", true, false}, // the Ingress still exists and still owns them
		{"nginx", false, false},
	} {
		p := contourMigrationProvider(tc.routerType, tc.migration)
		ns := p.AppNamespace("myapp")
		cm := cmfake.NewSimpleClientset(ingressOwnedCertificate(ns, "cert-web-domains"), ingressOwnedCertificate("rack-other", "cert-web"))
		p.CertManagerClient = cm

		require.NoError(t, p.releaseAdoptIngressCertificates("myapp"))

		c, err := cm.CertmanagerV1().Certificates(ns).Get(context.TODO(), "cert-web-domains", am.GetOptions{})
		require.NoError(t, err)
		kinds := []string{}
		for _, o := range c.OwnerReferences {
			kinds = append(kinds, o.Kind)
		}
		if tc.adopt {
			require.Equal(t, []string{"Other"}, kinds, "%+v", tc)
		} else {
			require.Equal(t, []string{"Ingress", "Other"}, kinds, "%+v", tc)
		}

		// another app's namespace is left alone
		other, err := cm.CertmanagerV1().Certificates("rack-other").Get(context.TODO(), "cert-web", am.GetOptions{})
		require.NoError(t, err)
		require.Len(t, other.OwnerReferences, 2)
	}
}
