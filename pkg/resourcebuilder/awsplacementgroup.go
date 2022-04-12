/*
Copyright 2022 Red Hat, Inc.
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

package resourcebuilder

import (
	machinev1 "github.com/openshift/machine-api-provider-aws/pkg/api/machine/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AWSPlacementGroup creates a new awsplacementgroup builder.
func AWSPlacementGroup() AWSPlacementGroupBuilder {
	return AWSPlacementGroupBuilder{}
}

// AWSPlacementGroupBuilder is used to build out a awsplacementgroup object.
type AWSPlacementGroupBuilder struct {
	generateName string
	name         string
	namespace    string
	labels       map[string]string
	spec         *machinev1.AWSPlacementGroupSpec
}

// Build builds a new awsplacementgroup based on the configuration provided.
func (p AWSPlacementGroupBuilder) Build() *machinev1.AWSPlacementGroup {
	pg := &machinev1.AWSPlacementGroup{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: p.generateName,
			Name:         p.name,
			Namespace:    p.namespace,
			Labels:       p.labels,
		},
	}

	if p.spec != nil {
		pg.Spec = *p.spec
	}

	return pg
}

// AsManaged sets the management state for the awsplacementgroup to managed.
func (p AWSPlacementGroupBuilder) AsManaged() AWSPlacementGroupBuilder {
	if p.spec == nil {
		p.spec = &machinev1.AWSPlacementGroupSpec{}
	}
	p.spec.ManagementSpec.ManagementState = machinev1.ManagedManagementState
	return p
}

// AsUnmanaged sets the management state for the awsplacementgroup to unmanaged.
func (p AWSPlacementGroupBuilder) AsUnmanaged() AWSPlacementGroupBuilder {
	if p.spec == nil {
		p.spec = &machinev1.AWSPlacementGroupSpec{}
	}
	p.spec.ManagementSpec.ManagementState = machinev1.UnmanagedManagementState
	return p
}

// AsPartitionType sets the managed group type to Partition.
func (p AWSPlacementGroupBuilder) AsPartitionType() AWSPlacementGroupBuilder {
	if p.spec == nil {
		p.spec = &machinev1.AWSPlacementGroupSpec{}
	}
	if p.spec.ManagementSpec.Managed == nil {
		p.spec.ManagementSpec.Managed = &machinev1.ManagedAWSPlacementGroup{}
	}

	p.spec.ManagementSpec.Managed.GroupType = machinev1.AWSPartitionPlacementGroupType
	return p
}

// AsPartitionTypeWithCount sets the managed group type to Partition as set the Partition count to the desired value.
func (p AWSPlacementGroupBuilder) AsPartitionTypeWithCount(count int32) AWSPlacementGroupBuilder {
	if p.spec == nil {
		p.spec = &machinev1.AWSPlacementGroupSpec{}
	}
	if p.spec.ManagementSpec.Managed == nil {
		p.spec.ManagementSpec.Managed = &machinev1.ManagedAWSPlacementGroup{}
	}

	p.spec.ManagementSpec.Managed.GroupType = machinev1.AWSPartitionPlacementGroupType
	p.spec.ManagementSpec.Managed.Partition = &machinev1.AWSPartitionPlacement{
		Count: count,
	}
	return p
}

// AsClusterType sets the managed group type to Cluster.
func (p AWSPlacementGroupBuilder) AsClusterType() AWSPlacementGroupBuilder {
	if p.spec == nil {
		p.spec = &machinev1.AWSPlacementGroupSpec{}
	}
	if p.spec.ManagementSpec.Managed == nil {
		p.spec.ManagementSpec.Managed = &machinev1.ManagedAWSPlacementGroup{}
	}
	p.spec.ManagementSpec.Managed.GroupType = machinev1.AWSClusterPlacementGroupType
	return p
}

// AsSpreadType sets the managed group type to Spread.
func (p AWSPlacementGroupBuilder) AsSpreadType() AWSPlacementGroupBuilder {
	if p.spec == nil {
		p.spec = &machinev1.AWSPlacementGroupSpec{}
	}
	if p.spec.ManagementSpec.Managed == nil {
		p.spec.ManagementSpec.Managed = &machinev1.ManagedAWSPlacementGroup{}
	}
	p.spec.ManagementSpec.Managed.GroupType = machinev1.AWSSpreadPlacementGroupType
	return p
}

// WithGenerateName sets the generateName for the machine builder.
func (m AWSPlacementGroupBuilder) WithGenerateName(generateName string) AWSPlacementGroupBuilder {
	m.generateName = generateName
	return m
}

// WithLabel sets the labels for the machine builder.
func (m AWSPlacementGroupBuilder) WithLabel(key, value string) AWSPlacementGroupBuilder {
	if m.labels == nil {
		m.labels = make(map[string]string)
	}

	m.labels[key] = value

	return m
}

// WithLabels sets the labels for the machine builder.
func (m AWSPlacementGroupBuilder) WithLabels(labels map[string]string) AWSPlacementGroupBuilder {
	m.labels = labels
	return m
}

// WithName sets the name for the machine builder.
func (m AWSPlacementGroupBuilder) WithName(name string) AWSPlacementGroupBuilder {
	m.name = name
	return m
}

// WithNamespace sets the namespace for the machine builder.
func (m AWSPlacementGroupBuilder) WithNamespace(namespace string) AWSPlacementGroupBuilder {
	m.namespace = namespace
	return m
}
