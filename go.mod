module github.com/jeffwecan/waypoint-plugin-nomad-traefik

go 1.14

require (
	github.com/golang/protobuf v1.4.3
	github.com/hashicorp/go-hclog v0.14.1
	github.com/hashicorp/nomad v1.0.2
	github.com/hashicorp/nomad/api v0.0.0-20210115191909-bcd4752fc902
	github.com/hashicorp/hcl/v2 v2.7.1-0.20210129140708-3000d85e32a9
	github.com/hashicorp/waypoint v0.2.0
	github.com/hashicorp/waypoint-plugin-sdk v0.0.0-20210125184501-4d87d2821275
	google.golang.org/protobuf v1.25.0
)

replace golang.org/x/sys => golang.org/x/sys v0.0.0-20200826173525-f9321e4c35a6

// replace github.com/hashicorp/waypoint-plugin-sdk => ../../waypoint-plugin-sdk
// umm IDK: https://github.com/kubernetes/client-go/issues/874
replace k8s.io/client-go => k8s.io/client-go v0.19.2

// without this replacement:
// ../../go/pkg/mod/github.com/hashicorp/nomad@v1.0.2/jobspec2/hcl_conversions.go:17:17: undefined: gohcl.Decoder
// replace	github.com/hashicorp/hcl => github.com/hashicorp/hcl v1.0.1-0.20201016140508-a07e7d50bbee
