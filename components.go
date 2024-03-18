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
	"ubuntu": "ubuntu-os-cloud/ubuntu-2204-lts",
	"debian": "debian-cloud/debian-11",
}

var machineSize = map[string]string{
	"small":  "e2-micro",
	"medium": "e2-small",
	"large":  "e2-medium",
}

var metadataStartupScripts = map[string]pulumi.String{
	"ubuntu": `#!/bin/bash
	
sudo apt update
sudo apt install -y nginx
sudo ufw allow 'Nginx HTTP'`,
	"debian": `#!/bin/bash
	
sudo apt update
sudo apt install -y nginx`,
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

			subnet_name := fmt.Sprintf("subnet-%v", k)

			// Create a subnet on the network.
			subnet, err := compute.NewSubnetwork(ctx, subnet_name, &compute.SubnetworkArgs{
				Name:        pulumi.String(v),
				IpCidrRange: pulumi.Sprintf("10.%v.0.0/20", (k+1)%100),
				Network:     network,
			}, pulumi.Parent(myFleet))
			if err != nil {
				return nil, err
			}

			myFleet.Subnets = append(myFleet.Subnets, subnet.Name)

		}
	}

	for k, v := range fleet.Machines {
		// Loop through and create all the machines using Instance Templates given the specs.

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
			MetadataStartupScript: metadataStartupScripts[v.OS],
			CanIpForward:          pulumi.Bool(false),
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

			subnet_name := fmt.Sprintf("subnet-%v", k)

			// Create a subnet on the network.
			_, err := compute.NewSubnetwork(ctx, subnet_name, &compute.SubnetworkArgs{
				Name:        pulumi.String(v),
				IpCidrRange: pulumi.Sprintf("10.%v.0.0/20", (k+1)%100),
				Network:     network,
			}, pulumi.Parent(myFleet))
			if err != nil {
				return nil, err
			}
		}
	}

	// Regional IP address for Regional External Forwarding Rule
	regionalAddress, err := compute.NewAddress(ctx, "regional-ip-address", &compute.AddressArgs{
		Name: pulumi.String("regional-ip-address"),
	})
	if err != nil {
		return nil, err
	}

	// allow access from health check ranges
	_, err = compute.NewFirewall(ctx, "health-check-fw-rule", &compute.FirewallArgs{
		Name:      pulumi.String("allow-health-checks"),
		Direction: pulumi.String("INGRESS"),
		Network:   network,
		SourceRanges: pulumi.StringArray{
			pulumi.String("130.211.0.0/22"),
			pulumi.String("35.191.0.0/16"),
		},
		Allows: compute.FirewallAllowArray{
			&compute.FirewallAllowArgs{
				Protocol: pulumi.String("tcp"),
				Ports: pulumi.StringArray{
					pulumi.String("80"),
				},
			},
		},
		TargetTags: pulumi.StringArray{
			pulumi.String("allow-health-check"),
		},
	}, pulumi.Parent(myFleet))
	if err != nil {
		return nil, err
	}

	// Health Check for Instance Groups
	health_check, err := compute.NewHealthCheck(ctx, "health-check", &compute.HealthCheckArgs{
		CheckIntervalSec:   pulumi.Int(5),
		TimeoutSec:         pulumi.Int(5),
		HealthyThreshold:   pulumi.Int(2),
		UnhealthyThreshold: pulumi.Int(10),
		HttpHealthCheck: &compute.HealthCheckHttpHealthCheckArgs{
			Port: pulumi.Int(80),
		},
	}, pulumi.Parent(myFleet))
	if err != nil {
		return nil, err
	}

	// Target Pool for Intance Groups
	instancePool, err := compute.NewTargetPool(ctx, "instance-pool", &compute.TargetPoolArgs{
		Name: pulumi.String("instance-pool"),
	})
	if err != nil {
		return nil, err
	}

	// Regional External Passthrough Network Load Balancer
	_, err = compute.NewForwardingRule(ctx, "external-network-lb", &compute.ForwardingRuleArgs{
		Name:                pulumi.String("external-network-lb"),
		IpAddress:           regionalAddress.SelfLink,
		IpProtocol:          pulumi.String("TCP"),
		LoadBalancingScheme: pulumi.String("EXTERNAL"),
		PortRange:           pulumi.String("80"),
		Target:              instancePool.SelfLink,
	})
	if err != nil {
		return nil, err
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
			Metadata: pulumi.Map{
				"foo": pulumi.Any("bar"),
			},
			MetadataStartupScript: metadataStartupScripts[v.OS],
			CanIpForward:          pulumi.Bool(false),
			Tags: pulumi.ToStringArray([]string{
				instanceTag,           // Required for web servers
				"allow-health-checks", // Required for health checks
			}),
		}, pulumi.Parent(myFleet))
		if err != nil {
			return nil, err
		}

		igm_name := fmt.Sprintf("%s-rigm-%v", v.OS, k)

		// Managed Instance Group
		_, err = compute.NewInstanceGroupManager(ctx, igm_name, &compute.InstanceGroupManagerArgs{
			Name: pulumi.String(igm_name),
			Zone: pulumi.String("us-central1-c"),
			NamedPorts: compute.InstanceGroupManagerNamedPortArray{
				&compute.InstanceGroupManagerNamedPortArgs{
					Name: pulumi.String("tcp"),
					Port: pulumi.Int(80),
				},
			},
			Versions: compute.InstanceGroupManagerVersionArray{
				&compute.InstanceGroupManagerVersionArgs{
					InstanceTemplate: instaceTemplate.SelfLinkUnique,
					Name:             pulumi.String(v.OS),
				},
			},
			BaseInstanceName: pulumi.String("fleet"),
			TargetSize:       pulumi.Int(v.Count),
			TargetPools: pulumi.StringArray{
				instancePool.SelfLink,
			},
			AutoHealingPolicies: &compute.InstanceGroupManagerAutoHealingPoliciesArgs{
				HealthCheck:     health_check.SelfLink,
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
