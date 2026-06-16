package domain

import (
	"context"
)

type VpnInterface interface {
	GetID() string
	GetName() string
	GetLocalIP() string
	GetInterfaceName() string
	GetStatus() string
}

type VpnDriver interface {
	Connect(ctx context.Context, node *VPNNode) (VpnInterface, error)
	Disconnect(ctx context.Context, node *VPNNode) error
}
