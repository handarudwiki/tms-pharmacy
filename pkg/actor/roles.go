package actor

// Roles from BRD §5. A user may hold several roles.
const (
	RoleAdmin           = "ORG_ADMIN"
	RoleTransportMgr    = "TRANSPORT_MANAGER"
	RoleDispatcher      = "DISPATCHER"
	RoleFleetMgr        = "FLEET_MANAGER"
	RoleDriver          = "DRIVER"
	RoleCarrierMgr      = "CARRIER_MANAGER"
	RoleCustomerService = "CUSTOMER_SERVICE"
	RoleAnalyst         = "TRANSPORT_ANALYST"
	RoleIntegration     = "INTEGRATION_ADMIN"
)

var AllRoles = []string{
	RoleAdmin, RoleTransportMgr, RoleDispatcher, RoleFleetMgr, RoleDriver, RoleCarrierMgr,
	RoleCustomerService, RoleAnalyst, RoleIntegration,
}
