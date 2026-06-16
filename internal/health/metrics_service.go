package health

import (
	"context"
	"log"
	"sync"
	"time"
	"vpn-to-proxy/internal/domain"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

type SystemMetricsService struct {
	mu          sync.RWMutex
	current     domain.SystemMetricSnapshot
	metricRepo  domain.SystemMetricRepository
	vpnRepo     domain.VPNNodeRepository
	proxyRepo   domain.ProxyRepository
	stopChan    chan struct{}
	prevNetIn   uint64
	prevNetOut  uint64
	lastNetTime time.Time
}

func NewSystemMetricsService(metricRepo domain.SystemMetricRepository, vpnRepo domain.VPNNodeRepository, proxyRepo domain.ProxyRepository) *SystemMetricsService {
	return &SystemMetricsService{
		metricRepo:  metricRepo,
		vpnRepo:     vpnRepo,
		proxyRepo:   proxyRepo,
		stopChan:    make(chan struct{}),
		lastNetTime: time.Now(),
	}
}

func (s *SystemMetricsService) Start(ctx context.Context) {
	// Collect initial metrics
	s.collect()
	go s.runMetricsCollector()
	go s.runMetricsHistorySaver()
}

func (s *SystemMetricsService) Stop() {
	close(s.stopChan)
}

func (s *SystemMetricsService) GetCurrent() domain.SystemMetricSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *SystemMetricsService) collect() {
	cpuPercent, err := cpu.Percent(0, false)
	var cpuVal float64
	if err == nil && len(cpuPercent) > 0 {
		cpuVal = cpuPercent[0]
	}

	virtualMem, err := mem.VirtualMemory()
	var ramVal float64
	if err == nil {
		ramVal = virtualMem.UsedPercent
	}

	diskStat, err := disk.Usage(".")
	var diskVal float64
	if err == nil {
		diskVal = diskStat.UsedPercent
	}

	// Calculate network bandwidth
	netIOCounters, err := net.IOCounters(false)
	var netInSpeed, netOutSpeed uint64
	if err == nil && len(netIOCounters) > 0 {
		now := time.Now()
		elapsed := now.Sub(s.lastNetTime).Seconds()
		if elapsed > 0 {
			currIn := netIOCounters[0].BytesRecv
			currOut := netIOCounters[0].BytesSent
			if s.prevNetIn > 0 && currIn >= s.prevNetIn {
				netInSpeed = uint64(float64(currIn-s.prevNetIn) / elapsed)
			}
			if s.prevNetOut > 0 && currOut >= s.prevNetOut {
				netOutSpeed = uint64(float64(currOut-s.prevNetOut) / elapsed)
			}
			s.prevNetIn = currIn
			s.prevNetOut = currOut
		}
		s.lastNetTime = now
	}

	// Active VPNs and Proxies
	var activeVPNs, activeProxies int
	vpns, err := s.vpnRepo.List()
	if err == nil {
		for _, v := range vpns {
			if v.Status == "connected" {
				activeVPNs++
			}
		}
	}
	proxies, err := s.proxyRepo.List()
	if err == nil {
		for _, p := range proxies {
			if p.Status == "running" {
				activeProxies++
			}
		}
	}

	s.mu.Lock()
	s.current = domain.SystemMetricSnapshot{
		CPUUsage:   cpuVal,
		RAMUsage:   ramVal,
		DiskUsage:  diskVal,
		NetIn:      netInSpeed,
		NetOut:     netOutSpeed,
		VPNCount:   activeVPNs,
		ProxyCount: activeProxies,
		Timestamp:  time.Now(),
	}
	s.mu.Unlock()
}

func (s *SystemMetricsService) runMetricsCollector() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.collect()
		}
	}
}

func (s *SystemMetricsService) runMetricsHistorySaver() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.mu.RLock()
			snapshot := s.current
			s.mu.RUnlock()

			s.metricRepo.Create(&snapshot)
			log.Println("[SystemMetricsService] Historical system metrics snapshot written to database")
		}
	}
}
