package model

type MDSPolicies struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	ServiceType     string              `json:"serviceType"`
	ResourceIds     []string            `json:"resourceIds,omitempty"`
	PermissionsSpec []MdsPermissionSpec `json:"permissionsSpec,omitempty"`
	NetworkSpecs    []MdsNetworkSpecs   `json:"networkSpecs,omitempty"`
}

type MdsPermissionSpec struct {
	Resource    string   `json:"resource"`
	Permissions []string `json:"permissions"`
	Role        string   `json:"role"`
}

type MdsNetworkSpecs struct {
	CIDR           string   `json:"cidr"`
	NetworkPortIds []string `json:"networkPortIds"`
}
