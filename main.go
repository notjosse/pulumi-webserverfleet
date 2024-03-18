package main

import (
	"github.com/pulumi/pulumi-gcp/sdk/v7/go/gcp/compute"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		// Import the program's configuration settings.
		cfg := config.New(ctx, "")

		instanceTag, err := cfg.Try("instanceTag")
		if err != nil {
			instanceTag = "web-server"
		}

		servicePort, err := cfg.Try("servicePort")
		if err != nil {
			servicePort = "80"
		}

		// Create a new network for the fleet
		network, err := compute.NewNetwork(ctx, "network", &compute.NetworkArgs{
			AutoCreateSubnetworks: pulumi.Bool(false),
		})
		if err != nil {
			return err
		}

		//Create a firewall allowing inbound access over ports 80 (for HTTP) and 22 (for SSH).
		firewall, err := compute.NewFirewall(ctx, "firewall", &compute.FirewallArgs{
			Network: network.SelfLink,
			Allows: compute.FirewallAllowArray{
				compute.FirewallAllowArgs{
					Protocol: pulumi.String("tcp"),
					Ports: pulumi.ToStringArray([]string{
						"22",
						servicePort,
					}),
				},
			},
			Direction: pulumi.String("INGRESS"),
			SourceRanges: pulumi.ToStringArray([]string{
				"0.0.0.0/0",
			}),
			TargetTags: pulumi.ToStringArray([]string{
				instanceTag,
			}),
		})
		if err != nil {
			return err
		}

		fleet, err := NewWebServerFleet2(ctx, "web-fleet", network.ID(), instanceTag, FleetSpec{
			Subnets: []string{
				"subnet-abc123",
				"subnet-abc123",
			},
			Machines: []MachineSpec{
				{OS: "ubuntu", Size: "small", Count: 1},
				{OS: "debian", Size: "medium", Count: 2},
			},
		}, pulumi.DependsOn([]pulumi.Resource{firewall}))

		// Exports
		ctx.Export("fleet-name", pulumi.String(fleet.Name))
		ctx.Export("fw-allow", firewall.Allows)
		return nil
	})
}
