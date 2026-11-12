---
title: "contour_migration"
description: "The contour_migration AWS rack parameter prepares a Rack to switch router_type between nginx and contour without downtime, by giving every App routes on both controllers."
slug: contour_migration
url: /configuration/rack-parameters/aws/contour_migration
---

# contour_migration

## Description

The `contour_migration` parameter lets you switch [`router_type`](/configuration/rack-parameters/aws/router_type) on a Rack with running Apps without taking them offline.

While `contour_migration` is `true`, the Rack installs Contour alongside nginx, whichever `router_type` is set, and every App deploy renders both an nginx Ingress and a Contour HTTPProxy for each Service. Both controllers can then serve every App, so moving the load balancer from one to the other doesn't leave any App without a route.

During the migration the HTTPProxy routes use the certificates that cert-manager issues for the nginx Ingress, so no certificate is reissued. When you turn `contour_migration` off on a Contour Rack, the next deploy of each App removes its nginx Ingress and the HTTPProxy takes over its certificates.

## Default Value

The default value is `false`.

## Switching to Contour without downtime

```bash
$ convox rack params set contour_migration=true -r rackName
```

Then redeploy every App (`convox deploy`, or promote its current release with `convox releases promote`), so each one gets its Contour routes. Check that each App's HTTPProxy is valid:

```bash
$ kubectl get httpproxies -A -l system=convox
```

Then move the load balancer to Envoy:

```bash
$ convox rack params set router_type=contour -r rackName
```

To go back, set `router_type=nginx`: the nginx routes are still in place. When you're satisfied, finish the migration and redeploy every App once more, which removes its nginx Ingress:

```bash
$ convox rack params set contour_migration=false -r rackName
```

Switching from Contour back to nginx works the same way.

## Viewing Current Configuration

```bash
$ convox rack params -r rackName
```

## Additional Information

This parameter is available on AWS Racks only. An App deploy fails with "Contour is not yet installed" until the Rack update that turns on `contour_migration` has finished.

## See Also

- [Ingress Router](/configuration/ingress-router)
- [router_type](/configuration/rack-parameters/aws/router_type)
