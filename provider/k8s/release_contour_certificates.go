package k8s

import (
	"context"

	"github.com/pkg/errors"
	am "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// releaseAdoptIngressCertificates keeps an app's certificates when its nginx Ingress goes away. On nginx, cert-manager's
// ingress-shim creates the cert-<service> Certificates and makes the Ingress their owner. The first Contour-only
// release (after router_type = contour with contour_migration off) renders Certificates with the same names, which
// apply updates in place, owner and all, then prunes the Ingress, and Kubernetes' garbage collector deletes the
// Certificates with it. The secrets stay, so TLS keeps working, but nothing renews them until the next release
// recreates the Certificates. Dropping the Ingress owner first lets the release adopt them.
func (p *Provider) releaseAdoptIngressCertificates(app string) error {
	if p.RouterType != "contour" || p.ContourMigration || p.CertManagerClient == nil {
		return nil
	}

	ns := p.AppNamespace(app)
	certs, err := p.CertManagerClient.CertmanagerV1().Certificates(ns).List(context.TODO(), am.ListOptions{})
	if err != nil {
		return errors.WithStack(err)
	}

	for i := range certs.Items {
		c := certs.Items[i]
		owners := []am.OwnerReference{}
		for _, o := range c.OwnerReferences {
			if o.Kind != "Ingress" {
				owners = append(owners, o)
			}
		}
		if len(owners) == len(c.OwnerReferences) {
			continue
		}
		c.OwnerReferences = owners
		if _, err := p.CertManagerClient.CertmanagerV1().Certificates(ns).Update(context.TODO(), &c, am.UpdateOptions{}); err != nil {
			return errors.Wrapf(err, "dropping the Ingress owner of certificate %s/%s", ns, c.Name)
		}
	}

	return nil
}
