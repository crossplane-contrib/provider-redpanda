// SPDX-FileCopyrightText: 2024 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/upjet/v2/pkg/controller"

	providerconfig "github.com/crossplane-contrib/provider-redpanda/internal/controller/namespaced/providerconfig"
	acl "github.com/crossplane-contrib/provider-redpanda/internal/controller/namespaced/redpanda/acl"
	cluster "github.com/crossplane-contrib/provider-redpanda/internal/controller/namespaced/redpanda/cluster"
	network "github.com/crossplane-contrib/provider-redpanda/internal/controller/namespaced/redpanda/network"
	schema "github.com/crossplane-contrib/provider-redpanda/internal/controller/namespaced/redpanda/schema"
	topic "github.com/crossplane-contrib/provider-redpanda/internal/controller/namespaced/redpanda/topic"
	user "github.com/crossplane-contrib/provider-redpanda/internal/controller/namespaced/redpanda/user"
	group "github.com/crossplane-contrib/provider-redpanda/internal/controller/namespaced/resource/group"
	assignment "github.com/crossplane-contrib/provider-redpanda/internal/controller/namespaced/role/assignment"
	registryacl "github.com/crossplane-contrib/provider-redpanda/internal/controller/namespaced/schema/registryacl"
	clusterserverless "github.com/crossplane-contrib/provider-redpanda/internal/controller/namespaced/serverless/cluster"
)

// Setup creates all controllers with the supplied logger and adds them to
// the supplied manager.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		providerconfig.Setup,
		acl.Setup,
		cluster.Setup,
		network.Setup,
		schema.Setup,
		topic.Setup,
		user.Setup,
		group.Setup,
		assignment.Setup,
		registryacl.Setup,
		clusterserverless.Setup,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}

// SetupGated creates all controllers with the supplied logger and adds them to
// the supplied manager gated.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		providerconfig.SetupGated,
		acl.SetupGated,
		cluster.SetupGated,
		network.SetupGated,
		schema.SetupGated,
		topic.SetupGated,
		user.SetupGated,
		group.SetupGated,
		assignment.SetupGated,
		registryacl.SetupGated,
		clusterserverless.SetupGated,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}
