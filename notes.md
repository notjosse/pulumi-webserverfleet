# Resource Notes

### Using a Global Forwarding Rule instead of External Passthrough Regional Forwarding Rule

Requires the following resources:
- Global Ip Address
- Global Forwarding Rule
- Target HTTP Proxy
- URL Map (pointing to backend service)
- Backend Service (that Points to the MIGs)
- Instance Templates
- Instance Group Manager (to create the MIG)
- Health Check
- Ingress Firewall Rules to Ports [22, 80] for SSH, HTTP Traffic and For Health Checks
 
Example of some resources:

        // Health Check for Instance Groups and Backend Service
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


		// Backend Service
        backend_service_name := fmt.Sprintf("%s-backend-service-%v", v.OS, k)

		fleetBackendService, err := compute.NewBackendService(ctx, backend_service_name, &compute.BackendServiceArgs{
		 	Name:                pulumi.String(backend_service_name),
		 	Protocol:            pulumi.String("HTTP"),
		 	PortName:            pulumi.String("tcp"),
		 	LoadBalancingScheme: pulumi.String("EXTERNAL"),
		 	TimeoutSec:          pulumi.Int(10),
		 	HealthChecks:        health_check.ID(),
		 	Backends: compute.BackendServiceBackendArray{
		 		&compute.BackendServiceBackendArgs{
		 			Group:          instanceGroupManager.InstanceGroup,
		 			BalancingMode:  pulumi.String("UTILIZATION"),
		 			MaxUtilization: pulumi.Float64(1),
		 			CapacityScaler: pulumi.Float64(1),
		 		},
		 	},
		 }, pulumi.Parent(myFleet))
		 if err != nil {
		 	return nil, err
		 }
        
         // URL Map
		 url_map_name := fmt.Sprintf("%s-url-map-%v", v.OS, k)

		 urlMap, err := compute.NewURLMap(ctx, url_map_name, &compute.URLMapArgs{
		 	Name:           pulumi.String(url_map_name),
		 	DefaultService: fleetBackendService.ID(),
		 	HostRules: compute.URLMapHostRuleArray{
		 		&compute.URLMapHostRuleArgs{
		 			Hosts: pulumi.StringArray{
		 				pulumi.String("*"),
		 			},
		 			PathMatcher: pulumi.String("allpaths"),
		 		},
		 	},
		 	PathMatchers: compute.URLMapPathMatcherArray{
		 		&compute.URLMapPathMatcherArgs{
		 			Name:           pulumi.String("allpaths"),
		 			DefaultService: fleetBackendService.ID(),
		 			PathRules: compute.URLMapPathMatcherPathRuleArray{
		 				&compute.URLMapPathMatcherPathRuleArgs{
		 					Paths: pulumi.StringArray{
		 						pulumi.String("/*"),
		 					},
		 					Service: fleetBackendService.ID(),
		 				},
		 			},
		 		},
		 	},
		 }, pulumi.Parent(myFleet))
		 if err != nil {
		 	return nil, err
		 }

		 // Target HTTP Proxy
         http_proxy_name := fmt.Sprintf("%s-http-proxy-%v", v.OS, k)

		 targetHttpProxy, err := compute.NewTargetHttpProxy(ctx, http_proxy_name, &compute.TargetHttpProxyArgs{
		 	Name:   pulumi.String(http_proxy_name),
		 	UrlMap: urlMap.ID(),
		 }, pulumi.Parent(myFleet))
		 if err != nil {
		 	return nil, err
		 }

		// forwarding rule
        fw_rule_name := fmt.Sprintf("%s-fw-rule-%v", v.OS, k)

		 _, err = compute.NewGlobalForwardingRule(ctx, fw_rule_name, &compute.GlobalForwardingRuleArgs{
		 	Name:                pulumi.String(fw_rule_name),
		 	IpProtocol:          pulumi.String("TCP"),
		 	LoadBalancingScheme: pulumi.String("EXTERNAL"),
		 	PortRange:           pulumi.String("80"),
		 	Target:              targetHttpProxy.ID(),
		 	IpAddress:           globalAddress.ID(),
		 }, pulumi.Parent(myFleet))
		 if err != nil {
		 	return nil, err
		 }