package k8s

import (
	"strings"
	"testing"

	"github.com/convox/convox/pkg/manifest"
	"github.com/convox/convox/pkg/mock"
	"github.com/convox/convox/pkg/structs"
	"github.com/convox/convox/pkg/templater"
	ktmpl "github.com/convox/convox/provider/k8s/template"
	"github.com/stretchr/testify/require"
	am "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakediscovery "k8s.io/client-go/discovery/fake"
	ktesting "k8s.io/client-go/testing"
	"sigs.k8s.io/yaml"
)

func contourMigrationProvider(routerType string, migration bool) *Provider {
	p := &Provider{
		Engine:           &mock.TestEngine{},
		Name:             "rack",
		Domain:           "example.org",
		DomainInternal:   "internal.example.org",
		RouterType:       routerType,
		ContourMigration: migration,
		ProxyProtocol:    true,
		DiscoveryClient: &fakediscovery.FakeDiscovery{Fake: &ktesting.Fake{Resources: []*am.APIResourceList{
			{GroupVersion: "projectcontour.io/v1", APIResources: []am.APIResource{{Name: "httpproxies", Kind: "HTTPProxy"}}},
		}}},
	}
	p.templater = templater.New(ktmpl.TemplatesFS)
	return p
}

func contourMigrationServices() manifest.Services {
	return manifest.Services{{
		Name:    "web",
		Port:    manifest.ServicePortScheme{Port: 3000, Scheme: "http"},
		Domains: []string{"app.example.com"},
		Timeout: 100,
	}}
}

type renderedObject struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		IngressClassName string `json:"ingressClassName"`
		TLS              []struct {
			SecretName string `json:"secretName"`
		} `json:"tls"`
		VirtualHost struct {
			TLS struct {
				SecretName string `json:"secretName"`
			} `json:"tls"`
		} `json:"virtualhost"`
	} `json:"spec"`
}

func renderedObjects(t *testing.T, data []byte) []renderedObject {
	t.Helper()
	objs := []renderedObject{}
	for _, doc := range strings.Split(string(data), "\n---\n") {
		if strings.TrimSpace(doc) == "" {
			continue
		}
		var o renderedObject
		require.NoError(t, yaml.Unmarshal([]byte(doc), &o), doc)
		if o.Kind == "" {
			continue
		}
		objs = append(objs, o)
	}
	return objs
}

func objectsByKind(objs []renderedObject) map[string][]renderedObject {
	m := map[string][]renderedObject{}
	for _, o := range objs {
		m[o.Kind] = append(m[o.Kind], o)
	}
	return m
}

func TestReleaseTemplateIngressContourMigrationRendersBothRoutes(t *testing.T) {
	for _, routerType := range []string{"nginx", "contour"} {
		t.Run(routerType, func(t *testing.T) {
			p := contourMigrationProvider(routerType, true)

			data, err := p.releaseTemplateIngress(&structs.App{Name: "myapp"}, contourMigrationServices(), structs.ReleasePromoteOptions{})
			require.NoError(t, err)

			kinds := objectsByKind(renderedObjects(t, data))

			require.Len(t, kinds["Ingress"], 1)
			require.Equal(t, "nginx", kinds["Ingress"][0].Spec.IngressClassName)

			require.Len(t, kinds["HTTPProxy"], 2)
			for _, hp := range kinds["HTTPProxy"] {
				require.Equal(t, "contour", hp.Spec.IngressClassName)
			}
			// The HTTPProxies use the secrets cert-manager's ingress-shim issues for the Ingress
			require.Equal(t, "cert-web", kinds["HTTPProxy"][0].Spec.VirtualHost.TLS.SecretName)
			require.Equal(t, "cert-web-domains", kinds["HTTPProxy"][1].Spec.VirtualHost.TLS.SecretName)
			require.Equal(t, "cert-web", kinds["Ingress"][0].Spec.TLS[0].SecretName)

			// ...and render no Certificate that would fight the ingress-shim's
			require.Empty(t, kinds["Certificate"])
		})
	}
}

func TestReleaseTemplateIngressInternalContourMigrationRendersBothRoutes(t *testing.T) {
	p := contourMigrationProvider("nginx", true)

	data, err := p.releaseTemplateIngressInternal(&structs.App{Name: "myapp"}, contourMigrationServices(), structs.ReleasePromoteOptions{})
	require.NoError(t, err)

	kinds := objectsByKind(renderedObjects(t, data))
	require.Len(t, kinds["Ingress"], 1)
	require.Equal(t, "nginx-internal", kinds["Ingress"][0].Spec.IngressClassName)
	require.NotEmpty(t, kinds["HTTPProxy"])
	for _, hp := range kinds["HTTPProxy"] {
		require.Equal(t, "contour-internal", hp.Spec.IngressClassName)
	}
	require.Empty(t, kinds["Certificate"])
}

func TestReleaseTemplateIngressWithoutContourMigration(t *testing.T) {
	t.Run("nginx renders only the Ingress", func(t *testing.T) {
		p := contourMigrationProvider("nginx", false)
		data, err := p.releaseTemplateIngress(&structs.App{Name: "myapp"}, contourMigrationServices(), structs.ReleasePromoteOptions{})
		require.NoError(t, err)
		kinds := objectsByKind(renderedObjects(t, data))
		require.Len(t, kinds["Ingress"], 1)
		require.Empty(t, kinds["HTTPProxy"])
	})

	t.Run("contour renders HTTPProxies with their own Certificates", func(t *testing.T) {
		p := contourMigrationProvider("contour", false)
		data, err := p.releaseTemplateIngress(&structs.App{Name: "myapp"}, contourMigrationServices(), structs.ReleasePromoteOptions{})
		require.NoError(t, err)
		kinds := objectsByKind(renderedObjects(t, data))
		require.Empty(t, kinds["Ingress"])
		require.Len(t, kinds["HTTPProxy"], 2)
		require.Len(t, kinds["Certificate"], 2)
	})
}

// convox_domain_tls_cert_disable only drops TLS for the rack's default host: custom domains keep it, as on nginx.
func TestReleaseTemplateHTTPProxyCustomDomainTLSWithDefaultHostCertDisabled(t *testing.T) {
	for _, migration := range []bool{false, true} {
		p := contourMigrationProvider("contour", migration)
		p.ConvoxDomainTLSCertDisable = true

		data, err := p.releaseTemplateIngress(&structs.App{Name: "myapp"}, contourMigrationServices(), structs.ReleasePromoteOptions{})
		require.NoError(t, err)
		kinds := objectsByKind(renderedObjects(t, data))

		require.Len(t, kinds["HTTPProxy"], 2)
		require.Equal(t, "web", kinds["HTTPProxy"][0].Metadata.Name)
		require.Empty(t, kinds["HTTPProxy"][0].Spec.VirtualHost.TLS.SecretName, "default host has no TLS")
		require.Equal(t, "cert-web-domains", kinds["HTTPProxy"][1].Spec.VirtualHost.TLS.SecretName, "migration=%v", migration)

		if migration {
			require.Empty(t, kinds["Certificate"])
		} else {
			require.Len(t, kinds["Certificate"], 1)
			require.Equal(t, "cert-web-domains", kinds["Certificate"][0].Metadata.Name)
		}
	}
}
