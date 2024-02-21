package main

import (
	"fmt"

	"github.com/pulumi/pulumi-gcp/sdk/v7/go/gcp/compute"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type MachineSpec struct {
	OS    string
	Size  string
	Count int
}

type FleetSpec struct {
	Subnets  []string
	Machines []MachineSpec
}

type WebServerFleet struct {
	pulumi.ResourceState
	Name        string
	Subnets     []pulumi.StringOutput
	InstanceIps []pulumi.StringPtrOutput
}

var machineOS = map[string]string{
	"ubuntu": "ubuntu-2204-lts-arm64",
	"debian": "debian-12-arm64",
}

var machineSize = map[string]string{
	"small":  "e2-micro",
	"medium": "e2-small",
	"large":  "e2-medium",
}

var metadataStartupScripts = map[string]pulumi.String{
	"ubuntu": `#!/bin/bash
	
sudo apt update
sudo apt install nginx
sudo ufw allow 'Nginx HTTP'`,
	"debian": `#!/bin/bash
	
sudo apt update
sudo apt -y install nginx`,
}

func NewWebServerFleet(ctx *pulumi.Context, name string, network pulumi.IDOutput, instanceTag string, fleet FleetSpec, opts ...pulumi.ResourceOption) (*WebServerFleet, error) {
	myFleet := &WebServerFleet{}
	err := ctx.RegisterComponentResource("main:utils:WebFleet", name, myFleet, opts...)
	if err != nil {
		return nil, err
	}

	existing_subnets := map[string]struct{}{}

	for k, v := range fleet.Subnets {
		// Ensure that the subnet doesn't already exist.
		if _, ok := existing_subnets[v]; ok {
			continue
		} else {

			// Add our subnet to the map to keep tab.
			existing_subnets[v] = struct{}{}

			// Create a subnet on the network.
			subnet, err := compute.NewSubnetwork(ctx, "subnet", &compute.SubnetworkArgs{
				Name:        pulumi.String(v),
				IpCidrRange: pulumi.Sprintf("10.%v.0.0/20", k+1),
				Network:     network,
			}, pulumi.Parent(myFleet))
			if err != nil {
				return nil, err
			}

			myFleet.Subnets = append(myFleet.Subnets, subnet.Name)

		}
	}

	for k, v := range fleet.Machines {
		// Loop through and create all the machines using Managed Instance Groups given the specs.

		template_name := fmt.Sprintf("%s-fleet-template", v.OS)

		instaceTemplate, err := compute.NewInstanceTemplate(ctx, template_name, &compute.InstanceTemplateArgs{
			MachineType: pulumi.String(machineSize[v.Size]),
			Disks: compute.InstanceTemplateDiskArray{
				&compute.InstanceTemplateDiskArgs{
					SourceImage: pulumi.String(machineOS[v.OS]),
					AutoDelete:  pulumi.Bool(true),
					DiskSizeGb:  pulumi.Int(10),
					Boot:        pulumi.Bool(true),
				},
			},
			NetworkInterfaces: compute.InstanceTemplateNetworkInterfaceArray{
				&compute.InstanceTemplateNetworkInterfaceArgs{
					Network:    network,
					Subnetwork: pulumi.String(fleet.Subnets[k]),
					AccessConfigs: compute.InstanceTemplateNetworkInterfaceAccessConfigArray{
						compute.InstanceTemplateNetworkInterfaceAccessConfigArgs{
							NatIp: nil,
							// NetworkTier: nil,
						},
					},
				},
			},
			ServiceAccount: compute.InstanceTemplateServiceAccountArgs{
				Scopes: pulumi.ToStringArray([]string{
					"https://www.googleapis.com/auth/cloud-platform",
				}),
			},
			Metadata: pulumi.Map{
				"foo": pulumi.Any("bar"),
			},
			CanIpForward: pulumi.Bool(true),
			Tags: pulumi.ToStringArray([]string{
				instanceTag,
			}),
		}, pulumi.Parent(myFleet))
		if err != nil {
			return nil, err
		}

		for i := 0; i < v.Count; i++ {
			instance_name := fmt.Sprintf("%s-instance-%d", v.OS, i)

			inst, err := compute.NewInstanceFromTemplate(ctx, instance_name, &compute.InstanceFromTemplateArgs{
				Zone:                   pulumi.String("us-central1-a"),
				SourceInstanceTemplate: instaceTemplate.SelfLinkUnique,
				CanIpForward:           pulumi.Bool(false),
				Labels: pulumi.StringMap{
					"my_key": pulumi.String("my_value"),
				},
				MetadataStartupScript: metadataStartupScripts[v.OS],
			}, pulumi.Parent(myFleet))
			if err != nil {
				return nil, err
			}

			myFleet.InstanceIps = append(myFleet.InstanceIps, inst.NetworkInterfaces.Index(pulumi.Int(0)).AccessConfigs().Index(pulumi.Int(0)).NatIp())
		}
	}
	myFleet.Name = name

	return myFleet, nil
}

func NewWebServerFleet2(ctx *pulumi.Context, name string, network pulumi.IDOutput, instanceTag string, fleet FleetSpec, opts ...pulumi.ResourceOption) (*WebServerFleet, error) {
	myFleet := &WebServerFleet{}
	err := ctx.RegisterComponentResource("main:utils:WebFleet2", name, myFleet, opts...)
	if err != nil {
		return nil, err
	}

	existing_subnets := map[string]struct{}{}

	for k, v := range fleet.Subnets {
		// Ensure that the subnet doesn't already exist.
		if _, ok := existing_subnets[v]; ok {
			continue
		} else {

			// Add our subnet to the map to keep tab.
			existing_subnets[v] = struct{}{}

			// Create a subnet on the network.
			_, err := compute.NewSubnetwork(ctx, "subnet", &compute.SubnetworkArgs{
				Name:        pulumi.String(v),
				IpCidrRange: pulumi.Sprintf("10.%v.0.0/20", k+1),
				Network:     network,
			}, pulumi.Parent(myFleet))
			if err != nil {
				return nil, err
			}
		}
	}

	// Health Check for Instance Groups
	autohealing, err := compute.NewHealthCheck(ctx, "autohealing", &compute.HealthCheckArgs{
		CheckIntervalSec:   pulumi.Int(5),
		TimeoutSec:         pulumi.Int(5),
		HealthyThreshold:   pulumi.Int(2),
		UnhealthyThreshold: pulumi.Int(10),
		HttpHealthCheck: &compute.HealthCheckHttpHealthCheckArgs{
			RequestPath: pulumi.String("/healthz"),
			Port:        pulumi.Int(80),
		},
	}, pulumi.Parent(myFleet))
	if err != nil {
		return nil, err
	}

	// Target Pool
	targetPool, err := compute.NewTargetPool(ctx, "target-pool", &compute.TargetPoolArgs{
		Region: pulumi.String("us-central1"),
	}, pulumi.Parent(myFleet))
	if err != nil {
		return nil, err
	}

	// Forwarding Rule
	_, err = compute.NewForwardingRule(ctx, "forwaring-rule", &compute.ForwardingRuleArgs{
		Target: targetPool.SelfLink,
	}, pulumi.Parent(myFleet))

	for k, v := range fleet.Machines {
		// Loop through and create all the machines using Managed Instance Groups given the specs.

		template_name := fmt.Sprintf("%s-fleet-template", v.OS)

		instaceTemplate, err := compute.NewInstanceTemplate(ctx, template_name, &compute.InstanceTemplateArgs{
			MachineType: pulumi.String(machineSize[v.Size]),
			Disks: compute.InstanceTemplateDiskArray{
				&compute.InstanceTemplateDiskArgs{
					SourceImage: pulumi.String(machineOS[v.OS]),
					AutoDelete:  pulumi.Bool(true),
					DiskSizeGb:  pulumi.Int(10),
					Boot:        pulumi.Bool(true),
				},
			},
			NetworkInterfaces: compute.InstanceTemplateNetworkInterfaceArray{
				&compute.InstanceTemplateNetworkInterfaceArgs{
					Network:    network,
					Subnetwork: pulumi.String(fleet.Subnets[k]),
				},
			},
			Metadata: pulumi.Map{
				"foo": pulumi.Any("bar"),
			},
			CanIpForward: pulumi.Bool(true),
			Tags: pulumi.ToStringArray([]string{
				instanceTag,       // Required for webservers
				"lb-health-check", //Required for Managed Instance Group health checks
			}),
		}, pulumi.Parent(myFleet))
		if err != nil {
			return nil, err
		}

		rigm_name := fmt.Sprintf("%s-rigm", v.OS)

		_, err = compute.NewRegionInstanceGroupManager(ctx, rigm_name, &compute.RegionInstanceGroupManagerArgs{
			BaseInstanceName: pulumi.String("fleet"),
			Region:           pulumi.String("us-central1"),
			DistributionPolicyZones: pulumi.StringArray{
				pulumi.String("us-central1-a"),
				pulumi.String("us-central1-c"),
			},
			Versions: compute.RegionInstanceGroupManagerVersionArray{
				&compute.RegionInstanceGroupManagerVersionArgs{
					InstanceTemplate: instaceTemplate.ID(),
				},
			},
			AllInstancesConfig: &compute.RegionInstanceGroupManagerAllInstancesConfigArgs{
				Metadata: pulumi.StringMap{
					"metadata_key": pulumi.String("metadata_value"),
				},
				Labels: pulumi.StringMap{
					"label_key": pulumi.String("label_value"),
				},
			},
			TargetPools: pulumi.StringArray{
				targetPool.SelfLink,
			},
			TargetSize: pulumi.Int(v.Count),
			NamedPorts: compute.RegionInstanceGroupManagerNamedPortArray{
				&compute.RegionInstanceGroupManagerNamedPortArgs{
					Name: pulumi.String("web-port"),
					Port: pulumi.Int(80),
				},
			},
			AutoHealingPolicies: &compute.RegionInstanceGroupManagerAutoHealingPoliciesArgs{
				HealthCheck:     autohealing.ID(),
				InitialDelaySec: pulumi.Int(300),
			},
		}, pulumi.Parent(myFleet))
		if err != nil {
			return nil, err
		}
	}

	myFleet.Name = name

	return myFleet, nil
}
