package proxy

import (
	"fmt"
	"net"
	"sync"
	"time"
	"vpn-to-proxy/internal/domain"
)

type PortAllocatorService struct {
	mu       sync.Mutex
	allocRepo domain.PortAllocationRepository
}

func NewPortAllocatorService(allocRepo domain.PortAllocationRepository) *PortAllocatorService {
	return &PortAllocatorService{
		allocRepo: allocRepo,
	}
}

// AllocatePort finds a free port in the range, reserves it, and returns it.
func (s *PortAllocatorService) AllocatePort(purpose string, targetID string, startPort, endPort int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for port := startPort; port <= endPort; port++ {
		if s.checkAvailability(port) {
			alloc := &domain.PortAllocation{
				Port:      port,
				Purpose:   purpose,
				TargetID:  targetID,
				Status:    "allocated",
				CreatedAt: time.Now(),
			}
			err := s.allocRepo.Create(alloc)
			if err == nil {
				return port, nil
			}
		}
	}

	return 0, fmt.Errorf("no free ports available in range %d - %d", startPort, endPort)
}

// ReservePort explicitly reserves a specific port.
func (s *PortAllocatorService) ReservePort(port int, purpose string, targetID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.checkAvailability(port) {
		return fmt.Errorf("port %d is not available", port)
	}

	alloc := &domain.PortAllocation{
		Port:      port,
		Purpose:   purpose,
		TargetID:  targetID,
		Status:    "reserved",
		CreatedAt: time.Now(),
	}
	return s.allocRepo.Create(alloc)
}

// ReleasePort releases a port registration.
func (s *PortAllocatorService) ReleasePort(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.allocRepo.Delete(port)
}

// CheckAvailability public endpoint.
func (s *PortAllocatorService) CheckAvailability(port int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.checkAvailability(port)
}

// Internal availability check: checks both DB and active socket listeners.
func (s *PortAllocatorService) checkAvailability(port int) bool {
	// 1. Check DB allocation
	alloc, err := s.allocRepo.GetByPort(port)
	if err == nil && alloc != nil {
		return false
	}

	// 2. Check local network socket
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false // socket already in use
	}
	ln.Close()

	return true
}
