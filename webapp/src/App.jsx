import React, { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { 
  Shield, 
  Settings, 
  Terminal, 
  Cpu, 
  HardDrive, 
  Activity, 
  Plus, 
  RefreshCw, 
  Trash2, 
  Power, 
  Play, 
  Square, 
  AlertCircle, 
  FileText, 
  Globe, 
  Lock, 
  User, 
  CheckCircle,
  Clock,
  Compass,
  Key,
  HelpCircle,
  ExternalLink,
  ChevronDown,
  Server,
  Heart,
  Wifi,
  Loader2,
  Database
} from 'lucide-react';
import ExpressVpnLocationPicker from './components/ExpressVpnLocationPicker';

const API_BASE = import.meta.env.DEV ? 'http://localhost:8080/api' : '/api';
const DEFAULT_STATS = {
  cpu_usage: 0,
  ram_usage: 0,
  active_vpn_count: 0,
  active_proxy_count: 0,
  health_grade: 'A',
  health_latency: 0,
  total_agents: 1,
  online_agents: 1
};

// Memoized Stat Card
const MetricCard = React.memo(({ title, value, icon: Icon, colorClass, subtitle }) => {
  return (
    <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5 flex items-center justify-between backdrop-blur-md hover:border-slate-700 transition-all duration-300 group">
      <div>
        <span className="text-xs text-slate-500 font-medium block">{title}</span>
        <span className="text-2xl font-bold tracking-tight text-slate-100 group-hover:text-white transition-colors mt-1 block">{value}</span>
        {subtitle && <span className="text-[10px] text-slate-400 mt-1 block">{subtitle}</span>}
      </div>
      <div className={`p-3 rounded-xl bg-slate-950/60 border border-slate-850 ${colorClass}`}>
        <Icon className="h-5 w-5" />
      </div>
    </div>
  );
});

// Memoized Area Chart for performance
const StatsChart = React.memo(({ data }) => {
  const points = data.length ? data : [{ cpu: 0, ram: 0 }];
  const makePath = (key) => points.map((d, i) => {
    const x = points.length === 1 ? 0 : (i / (points.length - 1)) * 100;
    const y = 100 - Math.max(0, Math.min(100, d[key] || 0));
    return `${i === 0 ? 'M' : 'L'} ${x.toFixed(2)} ${y.toFixed(2)}`;
  }).join(' ');

  return (
    <div className="h-32 w-full mt-4 min-w-0 rounded-xl border border-slate-800 bg-slate-950/50 p-3">
      <svg viewBox="0 0 100 100" preserveAspectRatio="none" className="h-full w-full overflow-visible">
        <path d={makePath('cpu')} fill="none" stroke="#8b5cf6" strokeWidth="2" vectorEffect="non-scaling-stroke" />
        <path d={makePath('ram')} fill="none" stroke="#14b8a6" strokeWidth="2" vectorEffect="non-scaling-stroke" />
      </svg>
    </div>
  );
});

export default function App() {
  const queryClient = useQueryClient();

  // Queries using TanStack Query
  const { data: vpns = [], refetch: refetchVpns } = useQuery({
    queryKey: ['vpns'],
    queryFn: () => fetch(`${API_BASE}/vpns`).then(res => res.json()),
    refetchInterval: 5000,
  });

  const { data: proxies = [], refetch: refetchProxies } = useQuery({
    queryKey: ['proxies'],
    queryFn: () => fetch(`${API_BASE}/proxies`).then(res => res.json()),
    refetchInterval: 5000,
  });

  const { data: credentials = [], refetch: refetchCredentials } = useQuery({
    queryKey: ['credentials'],
    queryFn: () => fetch(`${API_BASE}/vpn/credentials`).then(res => res.json()),
    refetchInterval: 10000,
  });

  const { data: agents = [], refetch: refetchAgents } = useQuery({
    queryKey: ['agents'],
    queryFn: () => fetch(`${API_BASE}/agents`).then(res => res.json()),
    refetchInterval: 5000,
  });

  const { data: stats = DEFAULT_STATS, refetch: refetchStats } = useQuery({
    queryKey: ['stats'],
    queryFn: () => fetch(`${API_BASE}/system/stats`).then(res => res.json()),
    refetchInterval: 3000,
  });

  const { data: logs = [], refetch: refetchLogs } = useQuery({
    queryKey: ['logs'],
    queryFn: () => fetch(`${API_BASE}/system/logs`).then(res => res.json()),
    refetchInterval: 4000,
  });

  const { data: commands = [], refetch: refetchCommands } = useQuery({
    queryKey: ['commands'],
    queryFn: () => fetch(`${API_BASE}/commands`).then(res => res.json()),
    refetchInterval: 3000,
  });

  const [customerToken, setCustomerToken] = useState(() => localStorage.getItem('customerToken') || '');
  const customerHeaders = useMemo(() => ({
    'Content-Type': 'application/json',
    ...(customerToken ? { Authorization: `Bearer ${customerToken}` } : {})
  }), [customerToken]);

  const { data: customerPlans = [] } = useQuery({
    queryKey: ['customerPlans'],
    queryFn: () => fetch(`${API_BASE}/customer/plans`).then(res => res.json()),
    refetchInterval: 15000,
  });

  const { data: customerProxies = [], refetch: refetchCustomerProxies } = useQuery({
    queryKey: ['customerProxies', customerToken],
    queryFn: () => fetch(`${API_BASE}/customer/proxies`, { headers: customerHeaders }).then(res => res.ok ? res.json() : []),
    enabled: !!customerToken,
    refetchInterval: 5000,
  });

  const { data: customerUsage = {}, refetch: refetchCustomerUsage } = useQuery({
    queryKey: ['customerUsage', customerToken],
    queryFn: () => fetch(`${API_BASE}/customer/subscription/usage`, { headers: customerHeaders }).then(res => res.ok ? res.json() : {}),
    enabled: !!customerToken,
    refetchInterval: 5000,
  });

  const { data: adminBilling = {}, refetch: refetchAdminBilling } = useQuery({
    queryKey: ['adminBilling'],
    queryFn: () => fetch(`${API_BASE}/admin/billing/overview`).then(res => res.json()),
    refetchInterval: 8000,
  });

  const { data: adminMetrics = {} } = useQuery({
    queryKey: ['adminProductionMetrics'],
    queryFn: () => fetch(`${API_BASE}/admin/metrics`).then(res => res.json()),
    refetchInterval: 5000,
  });

  const { data: customerSecurity = {}, refetch: refetchCustomerSecurity } = useQuery({
    queryKey: ['customerSecurity', customerToken],
    queryFn: () => fetch(`${API_BASE}/customer/security`, { headers: customerHeaders }).then(res => res.ok ? res.json() : {}),
    enabled: !!customerToken,
    refetchInterval: 8000,
  });

  const { data: adminAbuse = {}, refetch: refetchAdminAbuse } = useQuery({
    queryKey: ['adminAbuse'],
    queryFn: () => fetch(`${API_BASE}/admin/abuse/dashboard`).then(res => res.json()),
    refetchInterval: 8000,
  });

  const { data: blockedTargets = [], refetch: refetchBlockedTargets } = useQuery({
    queryKey: ['blockedTargets'],
    queryFn: () => fetch(`${API_BASE}/admin/blocked-targets`).then(res => res.json()),
    refetchInterval: 15000,
  });

  const { data: countryPools = [], refetch: refetchCountryPools } = useQuery({
    queryKey: ['countryPools'],
    queryFn: () => fetch(`${API_BASE}/pools`).then(res => res.json()),
    refetchInterval: 8000,
  });

  const { data: adminRouting = {}, refetch: refetchAdminRouting } = useQuery({
    queryKey: ['adminRouting'],
    queryFn: () => fetch(`${API_BASE}/admin/routing/dashboard`).then(res => res.json()),
    refetchInterval: 8000,
  });

  // Recharts Stats History
  const [statsHistory, setStatsHistory] = useState([]);
  const statsCPU = Math.round(stats.cpu_usage || 0);
  const statsRAM = Math.round(stats.ram_usage || 0);
  useEffect(() => {
    setStatsHistory(prev => {
      const next = [...prev, {
        time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
        cpu: statsCPU,
        ram: statsRAM,
      }];
      if (next.length > 20) next.shift();
      return next;
    });
  }, [statsCPU, statsRAM]);

  // Form & UI States
  const [vpnName, setVpnName] = useState('');
  const [vpnType, setVpnType] = useState('expressvpn');
  const [vpnProvider, setVpnProvider] = useState('ExpressVPN');
  const [vpnConfig, setVpnConfig] = useState('');
  
  const [proxyVPN, setProxyVPN] = useState('');
  const [proxyPort, setProxyPort] = useState('');
  const [proxyType, setProxyType] = useState('socks5');
  const [proxyUser, setProxyUser] = useState('');
  const [proxyPass, setProxyPass] = useState('');
  const [proxyExpiresHours, setProxyExpiresHours] = useState('12');
  const [proxyProvisionNotice, setProxyProvisionNotice] = useState(null);

  // ExpressVPN Credentials states
  const [selectedCred, setSelectedCred] = useState('');
  const [showAddCred, setShowAddCred] = useState(false);
  const [newCredName, setNewCredName] = useState('');
  const [newCredSecret, setNewCredSecret] = useState('');
  const [selectedLocation, setSelectedLocation] = useState(null);
  const [expressProtocol, setExpressProtocol] = useState('auto');
  
  const [autoCreateProxy, setAutoCreateProxy] = useState(true);
  const [autoProxyPort, setAutoProxyPort] = useState('8085');
  const [autoProxyType, setAutoProxyType] = useState('socks5');

  // Agent Form States
  const [newAgentName, setNewAgentName] = useState('');
  const [newAgentHost, setNewAgentHost] = useState('');
  const [newAgentIP, setNewAgentIP] = useState('');
  const [newAgentOS, setNewAgentOS] = useState('linux');
  const [isSubmittingAgent, setIsSubmittingAgent] = useState(false);
  const [agentTokenNotice, setAgentTokenNotice] = useState(null);

  // Validation States
  const [validationReport, setValidationReport] = useState(null);
  const [isValidating, setIsValidating] = useState(false);
  const [selectedValNode, setSelectedValNode] = useState('');

  // UI Navigation
  const [activeTab, setActiveTab] = useState('tunnels');
  const [errorMsg, setErrorMsg] = useState('');
  const [successMsg, setSuccessMsg] = useState('');
  const [logFilter, setLogFilter] = useState('');
  const [isSubmittingVPN, setIsSubmittingVPN] = useState(false);
  const [isSubmittingProxy, setIsSubmittingProxy] = useState(false);
  const [isSubmittingCred, setIsSubmittingCred] = useState(false);
  const [customerEmail, setCustomerEmail] = useState('');
  const [customerPassword, setCustomerPassword] = useState('');
  const [customerCountry, setCustomerCountry] = useState('');
  const [customerProxyType, setCustomerProxyType] = useState('socks5');
  const [customerRotation, setCustomerRotation] = useState('static');
  const [customerApiKeyName, setCustomerApiKeyName] = useState('');
  const [visibleCustomerSecret, setVisibleCustomerSecret] = useState(null);
  const [billingPlanName, setBillingPlanName] = useState('');
  const [billingCustomerId, setBillingCustomerId] = useState('');
  const [billingPlanId, setBillingPlanId] = useState('starter-v4');
  const [billingSubscriptionId, setBillingSubscriptionId] = useState('');
  const [billingPaymentStatus, setBillingPaymentStatus] = useState(null);
  const [whitelistIP, setWhitelistIP] = useState('');
  const [whitelistCIDR, setWhitelistCIDR] = useState('');
  const [blockedTargetType, setBlockedTargetType] = useState('domain');
  const [blockedTargetValue, setBlockedTargetValue] = useState('');
  const [blockedTargetReason, setBlockedTargetReason] = useState('');
  const [routingPoolCountry, setRoutingPoolCountry] = useState('');

  // Auto-populate default credential on load
  useEffect(() => {
    if (credentials.length > 0 && !selectedCred) {
      setSelectedCred(credentials[0].id);
    }
  }, [credentials.length, selectedCred]);

  // Toast notifier
  const notify = (type, msg) => {
    if (type === 'error') {
      setErrorMsg(msg);
      setTimeout(() => setErrorMsg(''), 6000);
    } else {
      setSuccessMsg(msg);
      setTimeout(() => setSuccessMsg(''), 6000);
    }
  };

  // Credentials creation
  const handleCreateCredential = async (e) => {
    e.preventDefault();
    if (!newCredName || !newCredSecret) return;
    setIsSubmittingCred(true);

    try {
      const res = await fetch(`${API_BASE}/vpn/credentials`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          provider: 'expressvpn',
          name: newCredName,
          secret: newCredSecret
        })
      });

      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || 'Failed to add credential');
      }

      const cred = await res.json();
      notify('success', `ExpressVPN Activation Code added: ${newCredName}`);
      setNewCredName('');
      setNewCredSecret('');
      setShowAddCred(false);
      queryClient.invalidateQueries({ queryKey: ['credentials'] });
      setSelectedCred(cred.id);
    } catch (err) {
      notify('error', err.message);
    } finally {
      setIsSubmittingCred(false);
    }
  };

  // VPN actions
  const handleCreateVPN = async (e) => {
    e.preventDefault();
    if (!vpnName) return;

    if (vpnType === 'expressvpn' || vpnType === 'expressvpn_mock') {
      if (!selectedCred) {
        notify('error', 'Please select or add an ExpressVPN Activation Code');
        return;
      }
      if (!selectedLocation) {
        notify('error', 'Please select a VPN Location alias');
        return;
      }
    }

    setIsSubmittingVPN(true);

    try {
      let node;
      if (vpnType === 'expressvpn' || vpnType === 'expressvpn_mock') {
        const res = await fetch(`${API_BASE}/vpn/expressvpn/nodes`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            name: vpnName,
            credentialId: selectedCred,
            locationAlias: selectedLocation.alias,
            locationDisplayName: selectedLocation.displayName,
            selectedCountry: selectedLocation.country,
            selectedRegion: selectedLocation.region,
            protocol: expressProtocol
          })
        });

        if (!res.ok) {
          const data = await res.json();
          throw new Error(data.error || 'Failed to save ExpressVPN Node');
        }

        node = await res.json();
        notify('success', `ExpressVPN Node ${vpnName} registered`);
      } else {
        const res = await fetch(`${API_BASE}/vpns`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            name: vpnName,
            provider: vpnProvider,
            type: vpnType,
            config_text: vpnConfig
          })
        });

        if (!res.ok) {
          const data = await res.json();
          throw new Error(data.error || 'Failed to create VPN node');
        }

        node = await res.json();
        notify('success', `VPN Node ${vpnName} registered successfully`);
      }

      // Automatically spawn tunnel & proxy if enabled
      if ((vpnType === 'expressvpn' || vpnType === 'expressvpn_mock') && autoCreateProxy && autoProxyPort) {
        notify('success', 'VPN Node Registered. Spawning Tunnel and Proxy Port...');
        const orchestratorRes = await fetch(`${API_BASE}/vpn/${node.id}/connect-and-create-proxy`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            proxyType: autoProxyType,
            port: parseInt(autoProxyPort),
            username: proxyUser || undefined,
            password: proxyPass || undefined,
            rotationMode: 'static'
          })
        });

        if (!orchestratorRes.ok) {
          const data = await orchestratorRes.json();
          throw new Error(data.error || 'VPN registered but failed to spawn tunnel/proxy');
        }

        notify('success', `Successfully spawned proxy port ${autoProxyPort} through ExpressVPN`);
      }

      setVpnName('');
      setVpnConfig('');
      setSelectedLocation(null);
      queryClient.invalidateQueries({ queryKey: ['vpns'] });
      queryClient.invalidateQueries({ queryKey: ['proxies'] });
    } catch (err) {
      notify('error', err.message);
    } finally {
      setIsSubmittingVPN(false);
    }
  };

  const handleConnectVPN = async (id) => {
    try {
      notify('success', 'Initiating VPN tunnel connection...');
      const res = await fetch(`${API_BASE}/vpns/${id}/connect`, { method: 'POST' });
      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || 'Failed to connect VPN');
      }
      notify('success', 'VPN tunnel connected successfully');
      queryClient.invalidateQueries({ queryKey: ['vpns'] });
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleDisconnectVPN = async (id) => {
    try {
      notify('success', 'Tearing down VPN tunnel...');
      const res = await fetch(`${API_BASE}/vpns/${id}/disconnect`, { method: 'POST' });
      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || 'Failed to disconnect VPN');
      }
      notify('success', 'VPN disconnected');
      queryClient.invalidateQueries({ queryKey: ['vpns'] });
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleDeleteVPN = async (id) => {
    if (!window.confirm('Delete this VPN? This will also stop and delete any associated proxy ports.')) return;
    try {
      const res = await fetch(`${API_BASE}/vpns/${id}`, { method: 'DELETE' });
      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || 'Failed to delete VPN');
      }
      notify('success', 'VPN node and linked proxies deleted');
      queryClient.invalidateQueries({ queryKey: ['vpns'] });
      queryClient.invalidateQueries({ queryKey: ['proxies'] });
    } catch (err) {
      notify('error', err.message);
    }
  };

  // Proxy actions
  const handleCreateProxy = async (e) => {
    e.preventDefault();
    if (!proxyPort) return;
    setIsSubmittingProxy(true);

    try {
      const res = await fetch(`${API_BASE}/proxies`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          vpn_node_id: proxyVPN || undefined,
          port: parseInt(proxyPort),
          type: proxyType,
          username: proxyUser || undefined,
          password: proxyPass || undefined,
          expires_hours: parseInt(proxyExpiresHours || '12', 10)
        })
      });

      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || 'Failed to create proxy');
      }

      const data = await res.json();
      const createdProxy = data.proxy || data;
      setProxyProvisionNotice({
        bindIP: createdProxy.bind_ip || '0.0.0.0',
        port: createdProxy.port,
        username: createdProxy.username,
        password: data.provisioned_password || createdProxy.password || '',
        expiresAt: data.expires_at || createdProxy.expires_at,
        expiresInHours: data.expires_in_hours || 12
      });
      notify('success', `Proxy configured on port ${proxyPort}`);
      setProxyPort('');
      setProxyUser('');
      setProxyPass('');
      setProxyExpiresHours('12');
      queryClient.invalidateQueries({ queryKey: ['proxies'] });
    } catch (err) {
      notify('error', err.message);
    } finally {
      setIsSubmittingProxy(false);
    }
  };

  const handleStartProxy = async (id) => {
    try {
      const res = await fetch(`${API_BASE}/proxies/${id}/start`, { method: 'POST' });
      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || 'Failed to start proxy');
      }
      notify('success', 'Proxy started successfully');
      queryClient.invalidateQueries({ queryKey: ['proxies'] });
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleStopProxy = async (id) => {
    try {
      const res = await fetch(`${API_BASE}/proxies/${id}/stop`, { method: 'POST' });
      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || 'Failed to stop proxy');
      }
      notify('success', 'Proxy stopped');
      queryClient.invalidateQueries({ queryKey: ['proxies'] });
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleDeleteProxy = async (id) => {
    if (!window.confirm('Delete this proxy mapping?')) return;
    try {
      const res = await fetch(`${API_BASE}/proxies/${id}`, { method: 'DELETE' });
      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || 'Failed to delete proxy');
      }
      notify('success', 'Proxy removed');
      queryClient.invalidateQueries({ queryKey: ['proxies'] });
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleCustomerAuth = async (mode) => {
    if (!customerEmail || !customerPassword) return;
    try {
      const res = await fetch(`${API_BASE}/customer/${mode}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: customerEmail, password: customerPassword })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Customer auth failed');
      localStorage.setItem('customerToken', data.token);
      setCustomerToken(data.token);
      notify('success', mode === 'register' ? 'Customer registered' : 'Customer logged in');
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleAllocateCustomerProxy = async () => {
    if (!customerToken) {
      notify('error', 'Login as customer first');
      return;
    }
    try {
      const res = await fetch(`${API_BASE}/customer/proxies/allocate`, {
        method: 'POST',
        headers: customerHeaders,
        body: JSON.stringify({
          country: customerCountry || undefined,
          type: customerProxyType,
          rotationMode: customerRotation
        })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Allocation failed');
      setVisibleCustomerSecret(data);
      notify('success', `Allocated proxy ${data.host}:${data.port}`);
      refetchCustomerProxies();
      refetchCustomerUsage();
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleRotateCustomerCredential = async (id) => {
    try {
      const res = await fetch(`${API_BASE}/customer/proxies/${id}/rotate-credential`, {
        method: 'POST',
        headers: customerHeaders
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Credential rotation failed');
      setVisibleCustomerSecret(data);
      notify('success', 'Proxy credential rotated');
      refetchCustomerProxies();
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleReleaseCustomerProxy = async (id) => {
    try {
      const res = await fetch(`${API_BASE}/customer/proxies/${id}`, {
        method: 'DELETE',
        headers: customerHeaders
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Release failed');
      notify('success', 'Customer proxy released');
      refetchCustomerProxies();
      refetchCustomerUsage();
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleCreateCustomerApiKey = async () => {
    try {
      const res = await fetch(`${API_BASE}/customer/api-keys`, {
        method: 'POST',
        headers: customerHeaders,
        body: JSON.stringify({ name: customerApiKeyName || 'default' })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'API key creation failed');
      setVisibleCustomerSecret({ apiKey: data.key });
      notify('success', 'Customer API key created');
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleCustomerExport = async (format) => {
    try {
      const res = await fetch(`${API_BASE}/customer/proxies/export?format=${format}`, {
        headers: customerHeaders
      });
      const body = await res.text();
      if (!res.ok) throw new Error(body || 'Export failed');
      const blob = new Blob([body], { type: format === 'json' ? 'application/json' : format === 'csv' ? 'text/csv' : 'text/plain' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `proxies.${format === 'txt' ? 'txt' : format}`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleAddWhitelist = async () => {
    if (!customerToken || (!whitelistIP && !whitelistCIDR)) return;
    try {
      const res = await fetch(`${API_BASE}/customer/ip-whitelist`, {
        method: 'POST',
        headers: customerHeaders,
        body: JSON.stringify({ ipAddress: whitelistIP || undefined, cidr: whitelistCIDR || undefined })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Whitelist update failed');
      setWhitelistIP('');
      setWhitelistCIDR('');
      notify('success', 'IP whitelist updated');
      refetchCustomerSecurity();
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleDeleteWhitelist = async (id) => {
    try {
      const res = await fetch(`${API_BASE}/customer/ip-whitelist/${id}`, {
        method: 'DELETE',
        headers: customerHeaders
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Whitelist delete failed');
      notify('success', 'Whitelist entry deleted');
      refetchCustomerSecurity();
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleAddBlockedTarget = async () => {
    if (!blockedTargetValue) return;
    try {
      const res = await fetch(`${API_BASE}/admin/blocked-targets`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ type: blockedTargetType, value: blockedTargetValue, reason: blockedTargetReason })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Blocked target update failed');
      setBlockedTargetValue('');
      setBlockedTargetReason('');
      notify('success', 'Blocked target added');
      refetchBlockedTargets();
      refetchAdminAbuse();
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleDeleteBlockedTarget = async (id) => {
    try {
      const res = await fetch(`${API_BASE}/admin/blocked-targets/${id}`, { method: 'DELETE' });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Blocked target delete failed');
      notify('success', 'Blocked target deleted');
      refetchBlockedTargets();
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleAbuseAction = async (path, message) => {
    try {
      const res = await fetch(`${API_BASE}${path}`, { method: 'POST' });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Abuse action failed');
      notify('success', message);
      refetchAdminAbuse();
      refetchAdminBilling();
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleCreateRoutingPool = async () => {
    if (!routingPoolCountry) return;
    try {
      const res = await fetch(`${API_BASE}/admin/routing/pools`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ country: routingPoolCountry, strategy: 'weighted_score' })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Pool creation failed');
      setRoutingPoolCountry('');
      notify('success', 'Routing pool created');
      refetchCountryPools();
      refetchAdminRouting();
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleSyncRoutingPools = async () => {
    try {
      const res = await fetch(`${API_BASE}/admin/routing/pools/sync`, { method: 'POST' });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Pool sync failed');
      notify('success', 'Routing pools synced');
      refetchCountryPools();
      refetchAdminRouting();
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleCreateBillingPlan = async () => {
    if (!billingPlanName) return;
    try {
      const res = await fetch(`${API_BASE}/admin/billing/plans`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          plan: {
            name: billingPlanName,
            description: 'Custom production plan',
            price: 99,
            currency: 'USD',
            max_proxies: 20,
            bandwidth_limit_gb: 200,
            concurrent_connections: 100,
            allowed_countries: '[]',
            status: 'active'
          },
          features: { support: 'standard' }
        })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Plan creation failed');
      setBillingPlanName('');
      notify('success', 'Billing plan created');
      refetchAdminBilling();
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleCreateBillingSubscription = async () => {
    if (!billingCustomerId || !billingPlanId) return;
    try {
      const res = await fetch(`${API_BASE}/admin/billing/subscriptions`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ customerId: billingCustomerId, planId: billingPlanId, days: 30, autoRenew: true, status: 'active' })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Subscription creation failed');
      notify('success', 'Subscription activated');
      refetchAdminBilling();
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleMarkInvoicePaid = async (id) => {
    try {
      const res = await fetch(`${API_BASE}/admin/billing/invoices/${id}/paid`, { method: 'POST' });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Invoice update failed');
      notify('success', 'Invoice marked paid');
      refetchAdminBilling();
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleGenerateInvoice = async () => {
    if (!billingCustomerId || !billingSubscriptionId) return;
    try {
      const res = await fetch(`${API_BASE}/admin/billing/invoices`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ customerId: billingCustomerId, subscriptionId: billingSubscriptionId })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Invoice generation failed');
      notify('success', 'Invoice generated');
      refetchAdminBilling();
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleVerifyInvoice = async (id) => {
    try {
      const res = await fetch(`${API_BASE}/admin/billing/invoices/${id}/verify`, { method: 'POST' });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Payment verification failed');
      notify('success', 'Payment verified');
      refetchAdminBilling();
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleRefundInvoice = async (id) => {
    try {
      const res = await fetch(`${API_BASE}/admin/billing/invoices/${id}/refund`, { method: 'POST' });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Refund failed');
      notify('success', 'Invoice refunded');
      refetchAdminBilling();
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleInvoicePaymentStatus = async (id) => {
    try {
      const res = await fetch(`${API_BASE}/admin/billing/invoices/${id}/payment-status`);
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Payment status failed');
      setBillingPaymentStatus(data);
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleQuickEditPlan = async () => {
    if (!billingPlanId) return;
    try {
      const res = await fetch(`${API_BASE}/admin/billing/plans/${billingPlanId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ description: 'Updated from admin billing dashboard' })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Plan update failed');
      notify('success', 'Plan updated');
    } catch (err) {
      notify('error', err.message);
    }
  };

  // Agent Actions
  const handleRegisterAgent = async (e) => {
    e.preventDefault();
    if (!newAgentName || !newAgentHost || !newAgentIP) return;
    setIsSubmittingAgent(true);

    try {
      const res = await fetch(`${API_BASE}/agents/register`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: newAgentName,
          hostname: newAgentHost,
          ip_address: newAgentIP,
          os: newAgentOS,
          version: 'v1.0.0'
        })
      });

      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || 'Failed to register agent');
      }
      const data = await res.json();

      setAgentTokenNotice({ agentId: data.agentId, token: data.token });
      notify('success', `Agent ${newAgentName} registered successfully`);
      setNewAgentName('');
      setNewAgentHost('');
      setNewAgentIP('');
      queryClient.invalidateQueries({ queryKey: ['agents'] });
    } catch (err) {
      notify('error', err.message);
    } finally {
      setIsSubmittingAgent(false);
    }
  };

  const handleRotateAgentToken = async (id) => {
    try {
      const res = await fetch(`${API_BASE}/agents/${id}/rotate-token`, { method: 'POST' });
      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || 'Failed to rotate token');
      }
      const data = await res.json();
      setAgentTokenNotice({ agentId: data.agentId, token: data.token });
      notify('success', 'Agent token rotated');
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleRevokeAgent = async (id) => {
    if (!window.confirm('Revoke this agent token and disconnect it?')) return;
    try {
      const res = await fetch(`${API_BASE}/agents/${id}/revoke`, { method: 'POST' });
      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || 'Failed to revoke agent');
      }
      notify('success', 'Agent revoked');
      queryClient.invalidateQueries({ queryKey: ['agents'] });
    } catch (err) {
      notify('error', err.message);
    }
  };

  const handleDeleteAgent = async (id) => {
    if (!window.confirm('Delete this agent node?')) return;
    try {
      const res = await fetch(`${API_BASE}/agents/${id}`, { method: 'DELETE' });
      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || 'Failed to delete agent');
      }
      notify('success', 'Agent node deleted');
      queryClient.invalidateQueries({ queryKey: ['agents'] });
    } catch (err) {
      notify('error', err.message);
    }
  };

  // Validation report parser
  const handleValidateNode = async () => {
    if (!selectedValNode) return;
    setIsValidating(true);
    setValidationReport(null);

    try {
      const res = await fetch(`${API_BASE}/vpn/${selectedValNode}/validate`, { method: 'POST' });
      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || 'Validation request failed');
      }
      const data = await res.json();
      
      let reportDetails = null;
      try {
        const parts = data.details.split('REPORT:\n');
        if (parts.length > 1) {
          reportDetails = JSON.parse(parts[1]);
        }
      } catch (err) {
        console.error('Failed to parse validation report json', err);
      }

      setValidationReport({
        status: data.status,
        details: data.details,
        report: reportDetails,
        timestamp: data.timestamp
      });
      notify('success', `ExpressVPN validation complete: ${data.status.toUpperCase()}`);
    } catch (err) {
      notify('error', err.message);
    } finally {
      setIsValidating(false);
    }
  };

  // Helpers
  const getVpnName = (id) => {
    const node = vpns.find(v => v.id === id);
    if (!node) return 'Unbound (System Default)';
    if (node.provider === 'expressvpn' || node.provider === 'ExpressVPN') {
      return `${node.name} (${node.locationDisplayName || node.locationAlias})`;
    }
    return node.name;
  };

  const getStatusColor = (status) => {
    switch (status) {
      case 'connected':
      case 'running':
      case 'healthy':
        return 'text-emerald-400 bg-emerald-400/10 border-emerald-500/20';
      case 'connecting':
        return 'text-amber-400 bg-amber-400/10 border-amber-500/20 animate-pulse';
      case 'failed':
      case 'error':
      case 'offline':
      case 'dead':
        return 'text-rose-400 bg-rose-400/10 border-rose-500/20';
      default:
        return 'text-slate-400 bg-slate-400/10 border-slate-500/20';
    }
  };

  const filteredLogs = useMemo(() => {
    return logs.filter(log => {
      if (!logFilter) return true;
      return log.action.toLowerCase().includes(logFilter.toLowerCase()) || 
             log.details.toLowerCase().includes(logFilter.toLowerCase());
    });
  }, [logs, logFilter]);

  const filteredCommands = useMemo(() => {
    return commands.filter(cmd => {
      if (!logFilter) return true;
      const haystack = `${cmd.id} ${cmd.agent_id} ${cmd.type} ${cmd.status} ${cmd.last_error || ''}`.toLowerCase();
      return haystack.includes(logFilter.toLowerCase());
    });
  }, [commands, logFilter]);

  // Virtualized row renderer
  const LogRow = useCallback(({ index, style }) => {
    const logItem = filteredLogs[index];
    if (!logItem) return null;
    return (
      <div style={style} className="flex gap-4 items-center text-[11px] font-mono leading-none py-1.5 px-2 hover:bg-slate-900/40 rounded transition-colors">
        <span className="text-slate-500 shrink-0">
          {new Date(logItem.timestamp).toLocaleTimeString()}
        </span>
        <span className={`px-2 py-0.5 rounded text-[9px] font-bold uppercase tracking-wider shrink-0 ${
          logItem.action.includes('ERROR') || logItem.action.includes('FAIL') ? 'bg-rose-600/20 text-rose-400 border border-rose-950' :
          logItem.action.includes('SUCCESS') || logItem.action.includes('START') || logItem.action.includes('CONNECT') ? 'bg-emerald-600/20 text-emerald-400 border border-emerald-950' :
          'bg-slate-800 text-slate-400 border border-slate-700/30'
        }`}>
          {logItem.action}
        </span>
        <span className="text-slate-300 truncate">{logItem.details}</span>
      </div>
    );
  }, [filteredLogs]);

  const CommandRow = useCallback(({ index, style }) => {
    const cmd = filteredCommands[index];
    if (!cmd) return null;
    return (
      <div style={style} className="grid grid-cols-[1.2fr_1fr_1fr_0.8fr_0.8fr_1.2fr] gap-3 items-center text-[11px] font-mono px-3 py-2 hover:bg-slate-900/50 border-b border-slate-900/60">
        <span className="truncate text-slate-300">{cmd.id}</span>
        <span className="truncate text-slate-400">{cmd.agent_id}</span>
        <span className="text-indigo-300 font-semibold">{cmd.type}</span>
        <span className={`px-2 py-0.5 rounded-full border text-[9px] uppercase w-fit ${getStatusColor(cmd.status)}`}>{cmd.status}</span>
        <span className="text-slate-400">{cmd.attempts || 0}/{cmd.max_attempts || 3}</span>
        <span className="truncate text-slate-500">{cmd.last_error || cmd.result || new Date(cmd.created_at).toLocaleTimeString()}</span>
      </div>
    );
  }, [filteredCommands]);

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 flex flex-col font-sans selection:bg-purple-600 selection:text-white">
      {/* Top Header Bar */}
      <header className="border-b border-slate-800 bg-slate-900/80 backdrop-blur-md sticky top-0 z-40 px-6 py-4 flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-gradient-to-tr from-violet-600 to-indigo-600 rounded-xl shadow-lg shadow-indigo-600/20">
            <Shield className="h-6 w-6 text-white" />
          </div>
          <div>
            <h1 className="text-xl font-bold tracking-tight bg-gradient-to-r from-white via-slate-200 to-slate-400 bg-clip-text text-transparent">
              VPN to Proxy Core
            </h1>
            <p className="text-xs text-slate-400 font-medium">Distributed Agent Cluster Platform</p>
          </div>
        </div>

        {/* Global Statistics Cards */}
        <div className="flex items-center gap-6">
          <div className="flex items-center gap-4 bg-slate-950/50 px-4 py-2 rounded-xl border border-slate-800">
            <div>
              <div className="text-[10px] text-slate-500 uppercase tracking-wider font-semibold">Tunnels</div>
              <div className="text-sm font-semibold flex items-center gap-1.5 mt-0.5">
                <span className="text-emerald-400">{vpns.filter(v => v.status === 'connected').length}</span>
                <span className="text-slate-600">/</span>
                <span className="text-slate-400">{vpns.length}</span>
              </div>
            </div>
            <div className="h-8 w-px bg-slate-800" />
            <div>
              <div className="text-[10px] text-slate-500 uppercase tracking-wider font-semibold">Proxies</div>
              <div className="text-sm font-semibold flex items-center gap-1.5 mt-0.5">
                <span className="text-emerald-400">{proxies.filter(p => p.status === 'running').length}</span>
                <span className="text-slate-600">/</span>
                <span className="text-slate-400">{proxies.length}</span>
              </div>
            </div>
            <div className="h-8 w-px bg-slate-800" />
            <div>
              <div className="text-[10px] text-slate-500 uppercase tracking-wider font-semibold">Agents</div>
              <div className="text-sm font-semibold flex items-center gap-1.5 mt-0.5">
                <span className="text-emerald-400">{stats.online_agents}</span>
                <span className="text-slate-600">/</span>
                <span className="text-slate-400">{stats.total_agents}</span>
              </div>
            </div>
          </div>

          <div className="flex items-center gap-4 bg-slate-950/50 px-4 py-2 rounded-xl border border-slate-800">
            <div className="flex items-center gap-2">
              <Cpu className="h-4 w-4 text-violet-400" />
              <div>
                <span className="text-[10px] text-slate-500 block uppercase tracking-wider font-semibold">CPU</span>
                <span className="text-sm font-semibold text-slate-200 mt-0.5 block">{Math.round(stats.cpu_usage)}%</span>
              </div>
            </div>
            <div className="h-8 w-px bg-slate-800" />
            <div className="flex items-center gap-2">
              <HardDrive className="h-4 w-4 text-indigo-400" />
              <div>
                <span className="text-[10px] text-slate-500 block uppercase tracking-wider font-semibold">RAM</span>
                <span className="text-sm font-semibold text-slate-200 mt-0.5 block">{Math.round(stats.ram_usage)}%</span>
              </div>
            </div>
          </div>
        </div>
      </header>

      {/* Messages / Notifications */}
      {errorMsg && (
        <div className="mx-6 mt-4 p-4 bg-rose-900/20 border border-rose-800/40 rounded-xl text-rose-300 text-sm flex items-center gap-3 animate-fadeIn">
          <AlertCircle className="h-5 w-5 flex-shrink-0 text-rose-400" />
          <span>{errorMsg}</span>
        </div>
      )}
      {successMsg && (
        <div className="mx-6 mt-4 p-4 bg-emerald-900/20 border border-emerald-800/40 rounded-xl text-emerald-300 text-sm flex items-center gap-3 animate-fadeIn">
          <CheckCircle className="h-5 w-5 flex-shrink-0 text-emerald-400" />
          <span>{successMsg}</span>
        </div>
      )}

      {/* Main Grid Content */}
      <main className="flex-1 p-6 grid grid-cols-1 xl:grid-cols-3 gap-6">
        
        {/* Left Side: Setup Forms Panel */}
        <div className="xl:col-span-1 flex flex-col gap-6">
          
          {/* Hardware Load Graph */}
          <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5 backdrop-blur-md">
            <div className="flex items-center justify-between mb-2">
              <div className="flex items-center gap-2">
                <Activity className="h-4 w-4 text-violet-400" />
                <h2 className="font-semibold text-sm">System Load Efficacy</h2>
              </div>
              <span className="text-xs text-slate-500">Real-time charts</span>
            </div>
            <StatsChart data={statsHistory} />
          </div>

          {/* Connect & Create Panel */}
          <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5 backdrop-blur-md">
            <h2 className="font-semibold text-sm mb-4 flex items-center gap-2">
              <Plus className="h-4 w-4 text-violet-400" /> Register VPN Connection
            </h2>
            <form onSubmit={handleCreateVPN} className="space-y-4">
              <div>
                <label className="text-xs text-slate-400 block mb-1">VPN Node Name</label>
                <input 
                  type="text" 
                  value={vpnName}
                  onChange={(e) => setVpnName(e.target.value)}
                  placeholder={vpnType === 'expressvpn' ? 'ExpressVPN-Vietnam-01' : 'US-WireGuard-01'} 
                  className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100 placeholder-slate-600 focus:outline-none focus:border-violet-600"
                  required
                />
              </div>

              <div>
                <label className="text-xs text-slate-400 block mb-1">Tunnel Engine</label>
                <select 
                  value={vpnType}
                  onChange={(e) => {
                    setVpnType(e.target.value);
                    if (e.target.value === 'expressvpn') {
                      setVpnProvider('ExpressVPN');
                    } else if (e.target.value === 'expressvpn_mock') {
                      setVpnProvider('ExpressVPN (Mock)');
                    } else if (e.target.value === 'wireguard') {
                      setVpnProvider('WireGuard');
                    } else {
                      setVpnProvider('Generic');
                    }
                  }}
                  className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100 focus:outline-none focus:border-violet-600"
                >
                  <option value="expressvpn">ExpressVPN (Official CLI)</option>
                  <option value="expressvpn_mock">ExpressVPN (Simulated Mock)</option>
                  <option value="wireguard">WireGuard (Linux NetNS)</option>
                  <option value="mock">Generic (Mock)</option>
                </select>
              </div>

              {(vpnType === 'wireguard' || vpnType === 'mock') && (
                <div>
                  <label className="text-xs text-slate-400 block mb-1">WireGuard Config (.conf)</label>
                  <textarea
                    value={vpnConfig}
                    onChange={(e) => setVpnConfig(e.target.value)}
                    placeholder={`[Interface]\nAddress = 10.14.0.2/16\nPrivateKey = ...\n\n[Peer]\nPublicKey = ...\nAllowedIPs = 0.0.0.0/0\nEndpoint = host:51820`}
                    rows={10}
                    className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100 placeholder-slate-600 focus:outline-none focus:border-violet-600 font-mono leading-5 resize-y"
                  />
                  <p className="mt-1 text-[11px] text-slate-500">
                    Dán toàn bộ nội dung file `.conf` vào đây. `DNS` sẽ được bỏ qua khi sanitize.
                  </p>
                </div>
              )}

              {/* ExpressVPN Configuration Form */}
              {(vpnType === 'expressvpn' || vpnType === 'expressvpn_mock') && (
                <div className="space-y-4 pt-2 border-t border-slate-800/50">
                  <div>
                    <div className="flex items-center justify-between mb-1">
                      <label className="text-xs text-slate-400">ExpressVPN Activation Code</label>
                      <button
                        type="button"
                        onClick={() => setShowAddCred(!showAddCred)}
                        className="text-[11px] text-violet-400 hover:text-violet-300 font-medium flex items-center gap-0.5"
                      >
                        {showAddCred ? 'Cancel' : '+ Add Code'}
                      </button>
                    </div>

                    {showAddCred ? (
                      <div className="bg-slate-950/80 border border-slate-800/80 rounded-xl p-3 space-y-2.5">
                        <input
                          type="text"
                          placeholder="Credential Name (e.g. Code-A)"
                          value={newCredName}
                          onChange={(e) => setNewCredName(e.target.value)}
                          className="w-full bg-slate-950 border border-slate-850 rounded-lg px-2.5 py-1.5 text-xs text-slate-100"
                        />
                        <input
                          type="password"
                          placeholder="Activation Code"
                          value={newCredSecret}
                          onChange={(e) => setNewCredSecret(e.target.value)}
                          className="w-full bg-slate-950 border border-slate-850 rounded-lg px-2.5 py-1.5 text-xs text-slate-100"
                        />
                        <button
                          type="button"
                          onClick={handleCreateCredential}
                          disabled={isSubmittingCred}
                          className="w-full bg-violet-600 text-white rounded-lg py-1.5 text-xs font-semibold hover:bg-violet-500 disabled:opacity-50"
                        >
                          Save Code
                        </button>
                      </div>
                    ) : (
                      <select
                        value={selectedCred}
                        onChange={(e) => setSelectedCred(e.target.value)}
                        className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100"
                      >
                        <option value="">-- Select Activation Code --</option>
                        {credentials.map(c => (
                          <option key={c.id} value={c.id}>{c.name} ({c.maskedSecret})</option>
                        ))}
                      </select>
                    )}
                  </div>

                  <div>
                    <label className="text-xs text-slate-400 block mb-1">Tunnel Protocol</label>
                    <select
                      value={expressProtocol}
                      onChange={(e) => setExpressProtocol(e.target.value)}
                      className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100 focus:outline-none focus:border-violet-600"
                    >
                      <option value="auto">Auto (Recommended)</option>
                      <option value="lightway_udp">Lightway - UDP</option>
                      <option value="lightway_tcp">Lightway - TCP</option>
                      <option value="openvpn_udp">OpenVPN - UDP</option>
                      <option value="openvpn_tcp">OpenVPN - TCP</option>
                    </select>
                  </div>

                  <div>
                    <label className="text-xs text-slate-400 block mb-1">Select VPN Exit Node</label>
                    <ExpressVpnLocationPicker
                      selectedAlias={selectedLocation?.alias}
                      onSelect={(loc) => {
                        setSelectedLocation(loc);
                        if (!vpnName || vpnName.startsWith('ExpressVPN-')) {
                          setVpnName(`ExpressVPN-${loc.country.replace(/\s+/g, '')}`);
                        }
                      }}
                    />
                    {selectedLocation && (
                      <div className="mt-2 text-xs bg-purple-500/10 border border-purple-500/20 rounded-xl p-2.5 flex items-center justify-between text-purple-300">
                        <span>Target Location:</span>
                        <span className="font-semibold text-slate-200">{selectedLocation.flag} {selectedLocation.displayName}</span>
                      </div>
                    )}
                  </div>

                  <div className="p-3 bg-slate-950/40 border border-slate-800/60 rounded-xl space-y-3">
                    <label className="flex items-center gap-2 cursor-pointer select-none">
                      <input
                        type="checkbox"
                        checked={autoCreateProxy}
                        onChange={(e) => setAutoCreateProxy(e.target.checked)}
                        className="rounded border-slate-800 text-purple-600 focus:ring-purple-500 bg-slate-950"
                      />
                      <span className="text-xs text-slate-300 font-medium">Connect VPN & Create Proxy automatically</span>
                    </label>

                    {autoCreateProxy && (
                      <div className="grid grid-cols-2 gap-2.5 pt-2 border-t border-slate-800/40">
                        <div>
                          <label className="text-[10px] text-slate-500 block mb-0.5">Proxy Port</label>
                          <input
                            type="number"
                            value={autoProxyPort}
                            onChange={(e) => setAutoProxyPort(e.target.value)}
                            placeholder="8085"
                            className="w-full bg-slate-950 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-100"
                          />
                        </div>
                        <div>
                          <label className="text-[10px] text-slate-500 block mb-0.5">Proxy Type</label>
                          <select
                            value={autoProxyType}
                            onChange={(e) => setAutoProxyType(e.target.value)}
                            className="w-full bg-slate-950 border border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-100"
                          >
                            <option value="socks5">SOCKS5</option>
                            <option value="http">HTTP</option>
                          </select>
                        </div>
                      </div>
                    )}
                  </div>
                </div>
              )}

              <button 
                type="submit" 
                disabled={isSubmittingVPN}
                className="w-full bg-gradient-to-r from-violet-600 to-indigo-600 hover:from-violet-500 hover:to-indigo-500 disabled:opacity-50 text-white rounded-xl py-2.5 font-semibold text-sm transition-all shadow-lg shadow-indigo-600/20"
              >
                {isSubmittingVPN ? 'Configuring Node...' : 'Register VPN Node'}
              </button>
            </form>
          </div>

          {/* Spawn Proxy Port Form */}
          <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5 backdrop-blur-md">
            <h2 className="font-semibold text-sm mb-4 flex items-center gap-2">
              <Plus className="h-4 w-4 text-violet-400" /> Spawn Proxy Port
            </h2>
            <form onSubmit={handleCreateProxy} className="space-y-4">
              <div>
                <label className="text-xs text-slate-400 block mb-1">Associate with VPN Tunnel</label>
                <select 
                  value={proxyVPN}
                  onChange={(e) => setProxyVPN(e.target.value)}
                  className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100 focus:outline-none focus:border-violet-600"
                >
                  <option value="">System Default (No VPN Loop)</option>
                  {vpns.map(v => (
                    <option key={v.id} value={v.id}>{getVpnName(v.id)}</option>
                  ))}
                </select>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="text-xs text-slate-400 block mb-1">Listening Port</label>
                  <input 
                    type="number" 
                    value={proxyPort}
                    onChange={(e) => setProxyPort(e.target.value)}
                    placeholder="8282" 
                    className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100 placeholder-slate-600 focus:outline-none focus:border-violet-600"
                  />
                </div>
                <div>
                  <label className="text-xs text-slate-400 block mb-1">Proxy Type</label>
                  <select 
                    value={proxyType}
                    onChange={(e) => setProxyType(e.target.value)}
                    className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100 focus:outline-none focus:border-violet-600"
                  >
                    <option value="socks5">SOCKS5</option>
                    <option value="http">HTTP/HTTPS</option>
                  </select>
                </div>
              </div>

              <div>
                <label className="text-xs text-slate-400 block mb-1">Rental Hours</label>
                <input
                  type="number"
                  min="1"
                  value={proxyExpiresHours}
                  onChange={(e) => setProxyExpiresHours(e.target.value)}
                  className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100 placeholder-slate-600 focus:outline-none focus:border-violet-600"
                />
                <p className="mt-1 text-[11px] text-slate-500">Leave username/password empty to auto-generate access for the rental.</p>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="text-xs text-slate-400 block mb-1">Username (Optional)</label>
                  <input 
                    type="text" 
                    value={proxyUser}
                    onChange={(e) => setProxyUser(e.target.value)}
                    placeholder="user" 
                    className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100 placeholder-slate-700 focus:outline-none focus:border-violet-600"
                  />
                </div>
                <div>
                  <label className="text-xs text-slate-400 block mb-1">Password (Optional)</label>
                  <input 
                    type="password" 
                    value={proxyPass}
                    onChange={(e) => setProxyPass(e.target.value)}
                    placeholder="pass" 
                    className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100 placeholder-slate-700 focus:outline-none focus:border-violet-600"
                  />
                </div>
              </div>

              {proxyProvisionNotice && (
                <div className="bg-emerald-950/20 border border-emerald-700/30 rounded-xl p-3 text-xs text-emerald-200 space-y-1">
                  <div className="font-semibold text-emerald-300">Provisioned proxy details</div>
                  <div>Host: use the server IP that hosts this app</div>
                  <div>Port: {proxyProvisionNotice.port}</div>
                  <div>Username: {proxyProvisionNotice.username}</div>
                  <div>Password: {proxyProvisionNotice.password || 'Using your custom password'}</div>
                  <div>Expires: {proxyProvisionNotice.expiresAt ? new Date(proxyProvisionNotice.expiresAt).toLocaleString() : `${proxyProvisionNotice.expiresInHours} hours from creation`}</div>
                </div>
              )}

              <button 
                type="submit" 
                disabled={isSubmittingProxy}
                className="w-full bg-gradient-to-r from-violet-600 to-indigo-600 hover:from-violet-500 hover:to-indigo-500 disabled:opacity-50 text-white rounded-xl py-2.5 font-semibold text-sm transition-all shadow-lg shadow-indigo-600/20"
              >
                {isSubmittingProxy ? 'Creating...' : 'Create Proxy Port'}
              </button>
            </form>
          </div>
        </div>

        {/* Right Side: Active Elements */}
        <div className="xl:col-span-2 flex flex-col gap-6">
          
          {/* Main Tab Controller */}
          <div className="flex gap-2 p-1 bg-slate-900 border border-slate-800 rounded-2xl w-fit">
            <button 
              onClick={() => setActiveTab('tunnels')}
              className={`px-4 py-2 text-sm font-semibold rounded-xl transition-all ${activeTab === 'tunnels' ? 'bg-violet-600 text-white shadow-md' : 'text-slate-400 hover:text-slate-200'}`}
            >
              VPN Tunnels ({vpns.length})
            </button>
            <button 
              onClick={() => setActiveTab('proxies')}
              className={`px-4 py-2 text-sm font-semibold rounded-xl transition-all ${activeTab === 'proxies' ? 'bg-violet-600 text-white shadow-md' : 'text-slate-400 hover:text-slate-200'}`}
            >
              Active Proxies ({proxies.length})
            </button>
            <button 
              onClick={() => setActiveTab('agents')}
              className={`px-4 py-2 text-sm font-semibold rounded-xl transition-all ${activeTab === 'agents' ? 'bg-violet-600 text-white shadow-md' : 'text-slate-400 hover:text-slate-200'}`}
            >
              Distributed Agents ({agents.length})
            </button>
            <button 
              onClick={() => setActiveTab('health')}
              className={`px-4 py-2 text-sm font-semibold rounded-xl transition-all ${activeTab === 'health' ? 'bg-violet-600 text-white shadow-md' : 'text-slate-400 hover:text-slate-200'}`}
            >
              System Health & Checks
            </button>
            <button 
              onClick={() => setActiveTab('commands')}
              className={`px-4 py-2 text-sm font-semibold rounded-xl transition-all ${activeTab === 'commands' ? 'bg-violet-600 text-white shadow-md' : 'text-slate-400 hover:text-slate-200'}`}
            >
              Command Audit Log
            </button>
            <button 
              onClick={() => setActiveTab('customer')}
              className={`px-4 py-2 text-sm font-semibold rounded-xl transition-all ${activeTab === 'customer' ? 'bg-violet-600 text-white shadow-md' : 'text-slate-400 hover:text-slate-200'}`}
            >
              Customer Product
            </button>
            <button
              onClick={() => setActiveTab('security')}
              className={`px-4 py-2 text-sm font-semibold rounded-xl transition-all ${activeTab === 'security' ? 'bg-violet-600 text-white shadow-md' : 'text-slate-400 hover:text-slate-200'}`}
            >
              Security
            </button>
            <button 
              onClick={() => setActiveTab('billing')}
              className={`px-4 py-2 text-sm font-semibold rounded-xl transition-all ${activeTab === 'billing' ? 'bg-violet-600 text-white shadow-md' : 'text-slate-400 hover:text-slate-200'}`}
            >
              Billing
            </button>
            <button
              onClick={() => setActiveTab('routing')}
              className={`px-4 py-2 text-sm font-semibold rounded-xl transition-all ${activeTab === 'routing' ? 'bg-violet-600 text-white shadow-md' : 'text-slate-400 hover:text-slate-200'}`}
            >
              Routing Monitor
            </button>
            <button
              onClick={() => setActiveTab('abuse')}
              className={`px-4 py-2 text-sm font-semibold rounded-xl transition-all ${activeTab === 'abuse' ? 'bg-violet-600 text-white shadow-md' : 'text-slate-400 hover:text-slate-200'}`}
            >
              Abuse Monitor
            </button>
          </div>

          {/* VPN Nodes Panel */}
          {activeTab === 'tunnels' && (
            <div className="flex flex-col gap-4">
              {vpns.length === 0 ? (
                <div className="bg-slate-900/30 border border-slate-800/50 rounded-2xl p-10 text-center text-slate-500">
                  <Compass className="h-10 w-10 mx-auto mb-2 text-slate-700" />
                  <p className="text-sm">No registered VPN nodes yet.</p>
                  <p className="text-xs text-slate-600 mt-1">Use the left configuration panel to add one.</p>
                </div>
              ) : (
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  {vpns.map(vpn => (
                    <div key={vpn.id} className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5 flex flex-col justify-between gap-4 backdrop-blur-md hover:border-slate-700 transition-all duration-300">
                      <div>
                        <div className="flex items-start justify-between">
                          <div>
                            <span className="text-xs font-semibold uppercase tracking-wider text-violet-400 mb-1 block">
                              {vpn.provider || 'Custom Provider'}
                            </span>
                            <h3 className="font-bold text-lg">{vpn.name}</h3>
                          </div>
                          <span className={`px-2.5 py-0.5 text-xs font-medium rounded-full border ${getStatusColor(vpn.status)}`}>
                            {vpn.status}
                          </span>
                        </div>

                        {/* Connection details */}
                        <div className="mt-4 space-y-2 border-t border-slate-800/60 pt-4">
                          <div className="flex justify-between text-xs">
                            <span className="text-slate-500">Driver Mode</span>
                            <span className="font-mono text-slate-300 capitalize">{vpn.type}</span>
                          </div>
                          {vpn.locationDisplayName && (
                            <div className="flex justify-between text-xs">
                              <span className="text-slate-500">Selected Node</span>
                              <span className="text-slate-300 font-semibold">{vpn.locationDisplayName}</span>
                            </div>
                          )}
                          {vpn.local_ip && (
                            <div className="flex justify-between text-xs">
                              <span className="text-slate-500">Local VPN IP</span>
                              <span className="font-mono text-slate-300">{vpn.local_ip}</span>
                            </div>
                          )}
                          {vpn.ip && (
                            <div className="flex justify-between text-xs">
                              <span className="text-slate-500">Public Exit IP</span>
                              <span className="font-mono text-emerald-400">{vpn.ip}</span>
                            </div>
                          )}
                          {vpn.country && (
                            <div className="flex justify-between text-xs">
                              <span className="text-slate-500">Location</span>
                              <span className="text-indigo-400 font-medium">{vpn.region ? `${vpn.region}, ` : ''}{vpn.country}</span>
                            </div>
                          )}
                          {vpn.isp && (
                            <div className="flex justify-between text-xs">
                              <span className="text-slate-500">ISP</span>
                              <span className="text-slate-400 truncate max-w-[150px]">{vpn.isp}</span>
                            </div>
                          )}
                        </div>
                      </div>

                      {/* Action buttons */}
                      <div className="flex gap-2 pt-2 border-t border-slate-800/40">
                        {vpn.status === 'connected' ? (
                          <button 
                            onClick={() => handleDisconnectVPN(vpn.id)}
                            className="flex-1 bg-rose-600/10 hover:bg-rose-600/20 text-rose-400 border border-rose-500/20 rounded-xl py-2 text-sm font-semibold flex items-center justify-center gap-2 transition-all"
                          >
                            <Power className="h-4 w-4" /> Disconnect
                          </button>
                        ) : (
                          <button 
                            onClick={() => handleConnectVPN(vpn.id)}
                            className="flex-1 bg-emerald-600/10 hover:bg-emerald-600/20 text-emerald-400 border border-emerald-500/20 rounded-xl py-2 text-sm font-semibold flex items-center justify-center gap-2 transition-all"
                          >
                            <Power className="h-4 w-4" /> Connect Tunnel
                          </button>
                        )}
                        <button 
                          onClick={() => handleDeleteVPN(vpn.id)}
                          className="bg-slate-950 hover:bg-rose-950/20 text-slate-500 hover:text-rose-400 border border-slate-800 hover:border-rose-900/30 rounded-xl px-3 transition-all"
                        >
                          <Trash2 className="h-4 w-4" />
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}

          {/* Proxies Table Panel */}
          {activeTab === 'proxies' && (
            <div className="bg-slate-900/50 border border-slate-800 rounded-2xl overflow-hidden backdrop-blur-md">
              <div className="overflow-x-auto">
                <table className="w-full text-left border-collapse">
                  <thead>
                    <tr className="border-b border-slate-800 bg-slate-900/60 text-xs font-semibold uppercase tracking-wider text-slate-400">
                      <th className="px-5 py-3.5">Proxy Port</th>
                      <th className="px-5 py-3.5">Type</th>
                      <th className="px-5 py-3.5">VPN Connection Bound</th>
                      <th className="px-5 py-3.5">Credentials</th>
                      <th className="px-5 py-3.5">Active Status</th>
                      <th className="px-5 py-3.5 text-right">Actions</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-800/60 text-sm">
                    {proxies.length === 0 ? (
                      <tr>
                        <td colSpan={6} className="px-5 py-10 text-center text-slate-500">
                          No active proxies mapped yet. Spawn one via the left panel.
                        </td>
                      </tr>
                    ) : (
                      proxies.map(prxy => (
                        <tr key={prxy.id} className="hover:bg-slate-800/20 transition-all">
                          <td className="px-5 py-4 font-mono font-bold text-violet-400">
                            {prxy.port}
                          </td>
                          <td className="px-5 py-4 uppercase font-semibold text-xs text-indigo-400">
                            {prxy.type}
                          </td>
                          <td className="px-5 py-4 font-medium text-slate-300">
                            {getVpnName(prxy.vpn_node_id)}
                          </td>
                          <td className="px-5 py-4 text-xs font-mono text-slate-400">
                            {prxy.username ? (
                              <div className="flex flex-col gap-1">
                                <span className="flex items-center gap-1.5 bg-slate-950 px-2 py-1 rounded-lg border border-slate-850 w-fit">
                                  <Lock className="h-3 w-3 text-slate-500" />
                                  <span>{prxy.username}:{prxy.password ? '••••••' : ''}</span>
                                </span>
                                {prxy.expires_at && (
                                  <span className="text-[10px] text-slate-500">
                                    Expires {new Date(prxy.expires_at).toLocaleString()}
                                  </span>
                                )}
                              </div>
                            ) : (
                              <span className="text-slate-600">None</span>
                            )}
                          </td>
                          <td className="px-5 py-4">
                            <span className={`px-2.5 py-0.5 text-xs font-medium rounded-full border ${getStatusColor(prxy.status)}`}>
                              {prxy.status}
                            </span>
                          </td>
                          <td className="px-5 py-4 text-right">
                            <div className="flex justify-end gap-2">
                              {prxy.status === 'running' ? (
                                <button 
                                  onClick={() => handleStopProxy(prxy.id)}
                                  className="p-1.5 bg-amber-600/10 hover:bg-amber-600/20 text-amber-400 border border-amber-500/20 rounded-lg transition-all"
                                  title="Stop Proxy"
                                >
                                  <Square className="h-4 w-4" />
                                </button>
                              ) : (
                                <button 
                                  onClick={() => handleStartProxy(prxy.id)}
                                  className="p-1.5 bg-emerald-600/10 hover:bg-emerald-600/20 text-emerald-400 border border-emerald-500/20 rounded-lg transition-all"
                                  title="Start Proxy"
                                >
                                  <Play className="h-4 w-4" />
                                </button>
                              )}
                              <button 
                                onClick={() => handleDeleteProxy(prxy.id)}
                                className="p-1.5 bg-slate-950 hover:bg-rose-950/20 text-slate-500 hover:text-rose-400 border border-slate-800 hover:border-rose-900/30 rounded-lg transition-all"
                                title="Delete"
                              >
                                <Trash2 className="h-4 w-4" />
                              </button>
                            </div>
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {activeTab === 'customer' && (
            <div className="flex flex-col gap-5">
              <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5">
                  <h3 className="font-semibold text-sm mb-4 flex items-center gap-2">
                    <User className="h-4 w-4 text-violet-400" /> Customer Access
                  </h3>
                  <div className="space-y-3">
                    <input
                      type="email"
                      value={customerEmail}
                      onChange={(e) => setCustomerEmail(e.target.value)}
                      placeholder="customer@example.com"
                      className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100"
                    />
                    <input
                      type="password"
                      value={customerPassword}
                      onChange={(e) => setCustomerPassword(e.target.value)}
                      placeholder="Password"
                      className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100"
                    />
                    <div className="flex gap-2">
                      <button onClick={() => handleCustomerAuth('login')} className="flex-1 bg-violet-600 hover:bg-violet-500 text-white rounded-xl py-2 text-sm font-semibold">
                        Login
                      </button>
                      <button onClick={() => handleCustomerAuth('register')} className="flex-1 bg-slate-950 hover:bg-slate-800 border border-slate-800 text-slate-200 rounded-xl py-2 text-sm font-semibold">
                        Register
                      </button>
                    </div>
                    {customerToken && (
                      <button
                        onClick={() => { localStorage.removeItem('customerToken'); setCustomerToken(''); }}
                        className="w-full border border-slate-800 text-slate-400 rounded-xl py-2 text-xs hover:text-slate-200"
                      >
                        Clear customer session
                      </button>
                    )}
                  </div>
                </div>

                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5 lg:col-span-2">
                  <h3 className="font-semibold text-sm mb-4 flex items-center gap-2">
                    <Database className="h-4 w-4 text-violet-400" /> Proxy Plans
                  </h3>
                  <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                    {customerPlans.map(plan => (
                      <div key={plan.id} className="bg-slate-950 border border-slate-800 rounded-xl p-4">
                        <div className="flex items-start justify-between">
                          <h4 className="font-bold text-slate-100">{plan.name}</h4>
                          <span className="text-xs text-emerald-400">${plan.price}</span>
                        </div>
                        <div className="mt-3 space-y-1 text-xs text-slate-400">
                          <div>Max proxies: <span className="text-slate-200">{plan.max_proxies}</span></div>
                          <div>Bandwidth: <span className="text-slate-200">{plan.bandwidth_limit_gb} GB</span></div>
                          <div>Concurrency: <span className="text-slate-200">{plan.concurrent_connections}</span></div>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              </div>

              {visibleCustomerSecret && (
                <div className="bg-emerald-950/25 border border-emerald-500/20 rounded-2xl p-4">
                  <div className="text-sm font-semibold text-emerald-300 mb-2">Visible once</div>
                  <code className="block bg-slate-950 border border-emerald-500/10 rounded-xl px-3 py-2 text-xs text-emerald-200 break-all">
                    {visibleCustomerSecret.apiKey
                      ? visibleCustomerSecret.apiKey
                      : `${visibleCustomerSecret.host}:${visibleCustomerSecret.port}:${visibleCustomerSecret.username}:${visibleCustomerSecret.password}`}
                  </code>
                </div>
              )}

              <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5">
                <div className="flex flex-col lg:flex-row gap-3 lg:items-end">
                  <div className="flex-1">
                    <label className="text-xs text-slate-400 block mb-1">Country filter</label>
                    <input value={customerCountry} onChange={(e) => setCustomerCountry(e.target.value)} placeholder="Vietnam" className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100" />
                  </div>
                  <div>
                    <label className="text-xs text-slate-400 block mb-1">Type</label>
                    <select value={customerProxyType} onChange={(e) => setCustomerProxyType(e.target.value)} className="bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100">
                      <option value="socks5">SOCKS5</option>
                      <option value="http">HTTP</option>
                    </select>
                  </div>
                  <div>
                    <label className="text-xs text-slate-400 block mb-1">Rotation</label>
                    <select value={customerRotation} onChange={(e) => setCustomerRotation(e.target.value)} className="bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100">
                      <option value="static">static</option>
                      <option value="sticky_30m">sticky_30m</option>
                      <option value="sticky_6h">sticky_6h</option>
                      <option value="sticky_24h">sticky_24h</option>
                      <option value="rotating">rotating</option>
                    </select>
                  </div>
                  <button onClick={handleAllocateCustomerProxy} className="bg-violet-600 hover:bg-violet-500 text-white rounded-xl px-4 py-2 text-sm font-semibold">
                    Allocate Proxy
                  </button>
                </div>
              </div>

              <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5">
                  <h3 className="font-semibold text-sm mb-3">Usage</h3>
                  <div className="space-y-2 text-sm text-slate-300">
                    <div className="flex justify-between"><span className="text-slate-500">Bandwidth in</span><span>{customerUsage.bandwidth_in || 0}</span></div>
                    <div className="flex justify-between"><span className="text-slate-500">Bandwidth out</span><span>{customerUsage.bandwidth_out || 0}</span></div>
                    <div className="flex justify-between"><span className="text-slate-500">Proxy count</span><span>{customerUsage.proxy_count || 0}</span></div>
                  </div>
                </div>
                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5">
                  <h3 className="font-semibold text-sm mb-3">API Keys</h3>
                  <div className="flex gap-2">
                    <input value={customerApiKeyName} onChange={(e) => setCustomerApiKeyName(e.target.value)} placeholder="automation" className="flex-1 bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100" />
                    <button onClick={handleCreateCustomerApiKey} className="bg-slate-950 border border-slate-800 hover:bg-slate-800 rounded-xl px-3 py-2 text-sm">Create</button>
                  </div>
                </div>
                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5">
                  <h3 className="font-semibold text-sm mb-3">Export</h3>
                  <div className="flex gap-2 text-sm">
                    <button onClick={() => handleCustomerExport('txt')} className="flex-1 text-center bg-slate-950 border border-slate-800 rounded-xl py-2 hover:bg-slate-800">TXT</button>
                    <button onClick={() => handleCustomerExport('csv')} className="flex-1 text-center bg-slate-950 border border-slate-800 rounded-xl py-2 hover:bg-slate-800">CSV</button>
                    <button onClick={() => handleCustomerExport('json')} className="flex-1 text-center bg-slate-950 border border-slate-800 rounded-xl py-2 hover:bg-slate-800">JSON</button>
                  </div>
                </div>
              </div>

              <div className="bg-slate-900/50 border border-slate-800 rounded-2xl overflow-hidden">
                <table className="w-full text-left">
                  <thead className="border-b border-slate-800 bg-slate-900/60 text-xs uppercase text-slate-400">
                    <tr>
                      <th className="px-5 py-3">Endpoint</th>
                      <th className="px-5 py-3">Country</th>
                      <th className="px-5 py-3">Rotation</th>
                      <th className="px-5 py-3">Status</th>
                      <th className="px-5 py-3 text-right">Actions</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-800 text-sm">
                    {customerProxies.map(item => (
                      <tr key={item.id}>
                        <td className="px-5 py-4 font-mono text-violet-300">{item.host}:{item.port}:{item.username}:******</td>
                        <td className="px-5 py-4 text-slate-300">{item.country || 'Any'}</td>
                        <td className="px-5 py-4 text-slate-300">{item.rotationMode}</td>
                        <td className="px-5 py-4 text-slate-300">{item.status}</td>
                        <td className="px-5 py-4">
                          <div className="flex justify-end gap-2">
                            <button onClick={() => handleRotateCustomerCredential(item.id)} className="px-3 py-1.5 rounded-lg border border-slate-800 hover:bg-slate-800 text-xs">Rotate credential</button>
                            <button onClick={() => handleReleaseCustomerProxy(item.id)} className="px-3 py-1.5 rounded-lg border border-rose-900/40 text-rose-300 hover:bg-rose-950/20 text-xs">Release</button>
                          </div>
                        </td>
                      </tr>
                    ))}
                    {customerProxies.length === 0 && (
                      <tr><td colSpan={5} className="px-5 py-8 text-center text-slate-500">No customer proxies allocated.</td></tr>
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {activeTab === 'security' && (
            <div className="flex flex-col gap-5">
              <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                <MetricCard title="Whitelist Entries" value={(customerSecurity.ip_whitelist || []).length} icon={Shield} colorClass="text-emerald-400" />
                <MetricCard title="Security Events" value={(customerSecurity.recent_abuse_events || []).length} icon={AlertCircle} colorClass="text-amber-400" />
                <MetricCard title="Active Proxies" value={customerProxies.length} icon={Lock} colorClass="text-indigo-400" />
              </div>

              <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5">
                  <div className="flex items-center justify-between mb-4">
                    <h3 className="font-semibold text-sm flex items-center gap-2">
                      <Shield className="h-4 w-4 text-emerald-400" /> Customer IP Whitelist
                    </h3>
                    <button onClick={() => refetchCustomerSecurity()} className="p-2 rounded-lg border border-slate-800 text-slate-400 hover:text-slate-100">
                      <RefreshCw className="h-4 w-4" />
                    </button>
                  </div>
                  <div className="grid grid-cols-1 md:grid-cols-[1fr_1fr_auto] gap-2 mb-4">
                    <input value={whitelistIP} onChange={(e) => setWhitelistIP(e.target.value)} placeholder="203.0.113.10" className="bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100" />
                    <input value={whitelistCIDR} onChange={(e) => setWhitelistCIDR(e.target.value)} placeholder="203.0.113.0/24" className="bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100" />
                    <button onClick={handleAddWhitelist} className="bg-violet-600 hover:bg-violet-500 text-white rounded-xl px-4 py-2 text-sm font-semibold">
                      Add
                    </button>
                  </div>
                  <div className="divide-y divide-slate-800 border border-slate-800 rounded-xl overflow-hidden">
                    {(customerSecurity.ip_whitelist || []).map(row => (
                      <div key={row.id} className="flex items-center justify-between gap-3 px-4 py-3 bg-slate-950/40">
                        <div>
                          <div className="font-mono text-sm text-slate-200">{row.ip_address || row.cidr}</div>
                          <div className="text-xs text-slate-500">{row.enabled ? 'enabled' : 'disabled'}</div>
                        </div>
                        <button onClick={() => handleDeleteWhitelist(row.id)} className="p-2 rounded-lg border border-rose-900/40 text-rose-300 hover:bg-rose-950/20">
                          <Trash2 className="h-4 w-4" />
                        </button>
                      </div>
                    ))}
                    {(customerSecurity.ip_whitelist || []).length === 0 && (
                      <div className="px-4 py-8 text-center text-sm text-slate-500">No whitelist entries.</div>
                    )}
                  </div>
                </div>

                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl overflow-hidden">
                  <div className="px-5 py-4 border-b border-slate-800 font-semibold text-sm flex items-center gap-2">
                    <AlertCircle className="h-4 w-4 text-amber-400" /> Recent Security Events
                  </div>
                  <div className="divide-y divide-slate-800 max-h-[420px] overflow-auto">
                    {(customerSecurity.recent_abuse_events || []).map(event => (
                      <div key={event.id} className="px-5 py-4">
                        <div className="flex items-center justify-between gap-3">
                          <span className="text-sm text-slate-200">{event.message}</span>
                          <span className={`px-2 py-0.5 rounded-full border text-[10px] uppercase ${getStatusColor(event.severity)}`}>{event.severity}</span>
                        </div>
                        <div className="mt-1 text-xs text-slate-500 font-mono">{event.rule_id || 'policy'} / {event.created_at ? new Date(event.created_at).toLocaleString() : ''}</div>
                      </div>
                    ))}
                    {(customerSecurity.recent_abuse_events || []).length === 0 && (
                      <div className="px-5 py-10 text-center text-sm text-slate-500">No security events for this customer.</div>
                    )}
                  </div>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'billing' && (
            <div className="flex flex-col gap-5">
              <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
                <MetricCard title="Total Customers" value={adminMetrics.total_customers || 0} icon={User} colorClass="text-violet-400" />
                <MetricCard title="Active Subscriptions" value={adminMetrics.active_subscriptions || 0} icon={CheckCircle} colorClass="text-emerald-400" />
                <MetricCard title="Monthly Revenue" value={`$${Math.round(adminMetrics.monthly_revenue || 0)}`} icon={Database} colorClass="text-indigo-400" />
                <MetricCard title="Pending Payments" value={adminMetrics.pending_payments || 0} icon={Clock} colorClass="text-amber-400" />
              </div>

              <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5">
                  <h3 className="font-semibold text-sm mb-3">Create Plan</h3>
                  <div className="flex gap-2">
                    <input value={billingPlanName} onChange={(e) => setBillingPlanName(e.target.value)} placeholder="Growth" className="flex-1 bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100" />
                    <button onClick={handleCreateBillingPlan} className="bg-violet-600 hover:bg-violet-500 text-white rounded-xl px-3 py-2 text-sm font-semibold">Create</button>
                  </div>
                </div>
                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5">
                  <h3 className="font-semibold text-sm mb-3">Activate Subscription</h3>
                  <div className="grid grid-cols-2 gap-2">
                    <input value={billingCustomerId} onChange={(e) => setBillingCustomerId(e.target.value)} placeholder="Customer ID" className="bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100" />
                    <select value={billingPlanId} onChange={(e) => setBillingPlanId(e.target.value)} className="bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100">
                      <option value="starter-v4">Starter</option>
                      <option value="professional-v4">Professional</option>
                      <option value="business-v4">Business</option>
                      <option value="enterprise-v4">Enterprise</option>
                    </select>
                  </div>
                  <div className="grid grid-cols-2 gap-2 mt-2">
                    <button onClick={handleCreateBillingSubscription} className="bg-slate-950 border border-slate-800 hover:bg-slate-800 rounded-xl py-2 text-sm">Activate</button>
                    <button onClick={handleQuickEditPlan} className="bg-slate-950 border border-slate-800 hover:bg-slate-800 rounded-xl py-2 text-sm">Edit Plan</button>
                  </div>
                </div>
                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5">
                  <h3 className="font-semibold text-sm mb-3">Invoice & Backups</h3>
                  <div className="flex gap-2 mb-2">
                    <input value={billingSubscriptionId} onChange={(e) => setBillingSubscriptionId(e.target.value)} placeholder="Subscription ID" className="flex-1 bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100" />
                    <button onClick={handleGenerateInvoice} className="bg-violet-600 hover:bg-violet-500 text-white rounded-xl px-3 py-2 text-sm font-semibold">Invoice</button>
                  </div>
                  <div className="flex gap-2">
                    <a href={`${API_BASE}/admin/backup/export?format=json`} className="flex-1 text-center bg-slate-950 border border-slate-800 rounded-xl py-2 hover:bg-slate-800 text-sm">JSON</a>
                    <a href={`${API_BASE}/admin/backup/export?format=zip`} className="flex-1 text-center bg-slate-950 border border-slate-800 rounded-xl py-2 hover:bg-slate-800 text-sm">ZIP</a>
                  </div>
                </div>
              </div>

              <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl overflow-hidden">
                  <div className="px-5 py-4 border-b border-slate-800 font-semibold text-sm">Subscriptions</div>
                  <table className="w-full text-left text-sm">
                    <thead className="text-xs uppercase text-slate-500 bg-slate-950/50">
                      <tr><th className="px-4 py-3">Customer</th><th className="px-4 py-3">Plan</th><th className="px-4 py-3">Status</th><th className="px-4 py-3">Expires</th></tr>
                    </thead>
                    <tbody className="divide-y divide-slate-800">
                      {(adminBilling.subscriptions || []).map(sub => (
                        <tr key={sub.id}>
                          <td className="px-4 py-3 font-mono text-xs text-slate-300">{sub.customer_id}</td>
                          <td className="px-4 py-3 text-slate-300">{sub.plan_id}</td>
                          <td className="px-4 py-3 text-slate-300">{sub.status}</td>
                          <td className="px-4 py-3 text-slate-400">{sub.expires_at ? new Date(sub.expires_at).toLocaleDateString() : ''}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>

                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl overflow-hidden">
                  <div className="px-5 py-4 border-b border-slate-800 font-semibold text-sm">Invoices</div>
                  <table className="w-full text-left text-sm">
                    <thead className="text-xs uppercase text-slate-500 bg-slate-950/50">
                      <tr><th className="px-4 py-3">Invoice</th><th className="px-4 py-3">Amount</th><th className="px-4 py-3">Status</th><th className="px-4 py-3 text-right">Action</th></tr>
                    </thead>
                    <tbody className="divide-y divide-slate-800">
                      {(adminBilling.invoices || []).map(invoice => (
                        <tr key={invoice.id}>
                          <td className="px-4 py-3 font-mono text-xs text-slate-300">{invoice.id}</td>
                          <td className="px-4 py-3 text-slate-300">{invoice.currency} {invoice.amount}</td>
                          <td className="px-4 py-3 text-slate-300">{invoice.status}</td>
                          <td className="px-4 py-3 text-right">
                            <div className="flex justify-end gap-2">
                              <button onClick={() => handleInvoicePaymentStatus(invoice.id)} className="px-2 py-1.5 rounded-lg border border-slate-800 text-slate-300 hover:bg-slate-800 text-xs">Status</button>
                              {invoice.status !== 'paid' && (
                                <button onClick={() => handleVerifyInvoice(invoice.id)} className="px-2 py-1.5 rounded-lg border border-indigo-800 text-indigo-300 hover:bg-indigo-950/20 text-xs">Verify</button>
                              )}
                              {invoice.status !== 'paid' && (
                                <button onClick={() => handleMarkInvoicePaid(invoice.id)} className="px-2 py-1.5 rounded-lg border border-emerald-800 text-emerald-300 hover:bg-emerald-950/20 text-xs">Paid</button>
                              )}
                              {invoice.status === 'paid' && (
                                <button onClick={() => handleRefundInvoice(invoice.id)} className="px-2 py-1.5 rounded-lg border border-rose-800 text-rose-300 hover:bg-rose-950/20 text-xs">Refund</button>
                              )}
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
              {billingPaymentStatus && (
                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-4 text-xs text-slate-300">
                  <span className="text-slate-500">Payment status:</span>{' '}
                  <code>{billingPaymentStatus.invoice_id} / {billingPaymentStatus.invoice_status} / {billingPaymentStatus.provider_status}</code>
                </div>
              )}
            </div>
          )}

          {activeTab === 'routing' && (
            <div className="flex flex-col gap-5">
              <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
                <MetricCard title="HA Score" value={adminRouting.ha_score || 0} icon={Heart} colorClass="text-emerald-400" />
                <MetricCard title="Country Pools" value={(adminRouting.pools || countryPools || []).length} icon={Globe} colorClass="text-violet-400" />
                <MetricCard title="Failovers" value={adminRouting.failover_count || 0} icon={RefreshCw} colorClass="text-amber-400" />
                <MetricCard title="Rotations" value={adminRouting.rotation_count || 0} icon={Activity} colorClass="text-indigo-400" />
              </div>

              <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5">
                  <h3 className="font-semibold text-sm mb-3 flex items-center gap-2">
                    <Globe className="h-4 w-4 text-violet-400" /> Pool Operations
                  </h3>
                  <div className="flex gap-2 mb-3">
                    <input value={routingPoolCountry} onChange={(e) => setRoutingPoolCountry(e.target.value)} placeholder="Vietnam" className="flex-1 bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100" />
                    <button onClick={handleCreateRoutingPool} className="bg-violet-600 hover:bg-violet-500 text-white rounded-xl px-3 py-2 text-sm font-semibold">Create</button>
                  </div>
                  <button onClick={handleSyncRoutingPools} className="w-full bg-slate-950 border border-slate-800 hover:bg-slate-800 rounded-xl py-2 text-sm flex items-center justify-center gap-2">
                    <RefreshCw className="h-4 w-4" /> Sync From Proxies
                  </button>
                </div>

                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5 lg:col-span-2">
                  <h3 className="font-semibold text-sm mb-3">Load Distribution</h3>
                  <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                    {(adminRouting.pools || countryPools || []).slice(0, 6).map(pool => (
                      <div key={pool.pool?.id || pool.pool?.country} className="bg-slate-950 border border-slate-800 rounded-xl p-4">
                        <div className="flex items-start justify-between gap-3">
                          <div>
                            <div className="font-bold text-slate-100">{pool.pool?.country || 'Unknown'}</div>
                            <div className="text-xs text-slate-500">{pool.health || 'unknown'}</div>
                          </div>
                          <span className="text-lg font-bold text-emerald-300">{pool.average_quality || 0}</span>
                        </div>
                        <div className="mt-3 space-y-1 text-xs text-slate-400">
                          <div className="flex justify-between"><span>Available</span><span className="text-slate-200">{pool.available_proxies || 0}/{pool.pool_size || 0}</span></div>
                          <div className="flex justify-between"><span>Sessions</span><span className="text-slate-200">{pool.active_sessions || 0}</span></div>
                          <div className="flex justify-between"><span>Agents</span><span className="text-slate-200">{pool.agent_redundancy || 0}</span></div>
                        </div>
                      </div>
                    ))}
                    {(adminRouting.pools || countryPools || []).length === 0 && (
                      <div className="text-sm text-slate-500 col-span-full text-center py-8">No routing pools synced yet.</div>
                    )}
                  </div>
                </div>
              </div>

              <div className="bg-slate-900/50 border border-slate-800 rounded-2xl overflow-hidden">
                <div className="px-5 py-4 border-b border-slate-800 font-semibold text-sm">Country Pools</div>
                <table className="w-full text-left text-sm">
                  <thead className="text-xs uppercase text-slate-500 bg-slate-950/50">
                    <tr><th className="px-4 py-3">Country</th><th className="px-4 py-3">Health</th><th className="px-4 py-3">Quality</th><th className="px-4 py-3">Available</th><th className="px-4 py-3">Redundancy</th></tr>
                  </thead>
                  <tbody className="divide-y divide-slate-800">
                    {(adminRouting.pools || countryPools || []).map(pool => (
                      <tr key={pool.pool?.id || pool.pool?.country}>
                        <td className="px-4 py-3 font-semibold text-slate-200">{pool.pool?.country || 'Unknown'}</td>
                        <td className="px-4 py-3 text-slate-300">{pool.health}</td>
                        <td className="px-4 py-3 text-slate-300">{pool.average_quality || 0}</td>
                        <td className="px-4 py-3 text-slate-300">{pool.available_proxies || 0}/{pool.pool_size || 0}</td>
                        <td className="px-4 py-3 text-slate-300">{pool.agent_redundancy || 0} agents</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              <div className="bg-slate-900/50 border border-slate-800 rounded-2xl overflow-hidden">
                <div className="px-5 py-4 border-b border-slate-800 font-semibold text-sm">Routing Events</div>
                <div className="divide-y divide-slate-800 max-h-[360px] overflow-auto">
                  {(adminRouting.events || []).map(event => (
                    <div key={event.id} className="px-5 py-4">
                      <div className="flex items-center justify-between gap-3">
                        <span className="text-sm text-slate-200">{event.message}</span>
                        <span className="text-xs text-indigo-300">{event.action}</span>
                      </div>
                      <div className="mt-1 text-xs text-slate-500 font-mono">
                        {event.customer_id || 'system'} / {event.proxy_id || event.pool_id || 'n/a'} / {event.created_at ? new Date(event.created_at).toLocaleString() : ''}
                      </div>
                    </div>
                  ))}
                  {(adminRouting.events || []).length === 0 && (
                    <div className="px-5 py-10 text-center text-sm text-slate-500">No routing events recorded.</div>
                  )}
                </div>
              </div>
            </div>
          )}

          {activeTab === 'abuse' && (
            <div className="flex flex-col gap-5">
              <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
                <MetricCard title="Failed Auth" value={adminAbuse.failed_auth_attempts || 0} icon={Lock} colorClass="text-amber-400" />
                <MetricCard title="Blocked Targets" value={adminAbuse.blocked_target_attempts || 0} icon={Shield} colorClass="text-rose-400" />
                <MetricCard title="Suspended Proxies" value={adminAbuse.suspended_proxies || 0} icon={Power} colorClass="text-indigo-400" />
                <MetricCard title="Bandwidth Spikes" value={adminAbuse.bandwidth_spikes || 0} icon={Activity} colorClass="text-emerald-400" />
              </div>

              <div className="grid grid-cols-1 xl:grid-cols-3 gap-4">
                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5">
                  <h3 className="font-semibold text-sm mb-3">Blocked Target Policy</h3>
                  <div className="grid grid-cols-1 gap-2 mb-4">
                    <select value={blockedTargetType} onChange={(e) => setBlockedTargetType(e.target.value)} className="bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100">
                      <option value="domain">Domain</option>
                      <option value="ip">IP</option>
                      <option value="cidr">CIDR</option>
                      <option value="port">Port</option>
                    </select>
                    <input value={blockedTargetValue} onChange={(e) => setBlockedTargetValue(e.target.value)} placeholder="blocked.example or 25" className="bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100" />
                    <input value={blockedTargetReason} onChange={(e) => setBlockedTargetReason(e.target.value)} placeholder="Reason" className="bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-100" />
                    <button onClick={handleAddBlockedTarget} className="bg-violet-600 hover:bg-violet-500 text-white rounded-xl px-4 py-2 text-sm font-semibold">Add Target</button>
                  </div>
                  <div className="divide-y divide-slate-800 border border-slate-800 rounded-xl overflow-hidden">
                    {blockedTargets.map(target => (
                      <div key={target.id} className="flex items-center justify-between gap-3 px-4 py-3 bg-slate-950/40">
                        <div>
                          <div className="font-mono text-sm text-slate-200">{target.type}:{target.value}</div>
                          <div className="text-xs text-slate-500">{target.reason || 'policy'}</div>
                        </div>
                        <button onClick={() => handleDeleteBlockedTarget(target.id)} className="p-2 rounded-lg border border-rose-900/40 text-rose-300 hover:bg-rose-950/20">
                          <Trash2 className="h-4 w-4" />
                        </button>
                      </div>
                    ))}
                    {blockedTargets.length === 0 && (
                      <div className="px-4 py-8 text-center text-sm text-slate-500">No blocked targets.</div>
                    )}
                  </div>
                </div>

                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl overflow-hidden xl:col-span-2">
                  <div className="px-5 py-4 border-b border-slate-800 font-semibold text-sm">High Risk Customers</div>
                  <table className="w-full text-left text-sm">
                    <thead className="text-xs uppercase text-slate-500 bg-slate-950/50">
                      <tr><th className="px-4 py-3">Customer</th><th className="px-4 py-3">Score</th><th className="px-4 py-3">Level</th><th className="px-4 py-3 text-right">Actions</th></tr>
                    </thead>
                    <tbody className="divide-y divide-slate-800">
                      {(adminAbuse.high_risk_customers || []).map(row => (
                        <tr key={row.customer_id}>
                          <td className="px-4 py-3 font-mono text-xs text-slate-300">{row.customer_id}</td>
                          <td className="px-4 py-3 text-slate-200">{row.score}</td>
                          <td className="px-4 py-3 text-slate-300">{row.level}</td>
                          <td className="px-4 py-3">
                            <div className="flex justify-end gap-2">
                              <button onClick={() => handleAbuseAction(`/admin/abuse/customers/${row.customer_id}/suspend`, 'Customer suspended')} className="px-2 py-1.5 rounded-lg border border-amber-800 text-amber-300 hover:bg-amber-950/20 text-xs">Suspend</button>
                              <button onClick={() => handleAbuseAction(`/admin/abuse/customers/${row.customer_id}/clear-risk`, 'Risk cleared')} className="px-2 py-1.5 rounded-lg border border-slate-800 text-slate-300 hover:bg-slate-800 text-xs">Clear Risk</button>
                            </div>
                          </td>
                        </tr>
                      ))}
                      {(adminAbuse.high_risk_customers || []).length === 0 && (
                        <tr><td colSpan={4} className="px-5 py-8 text-center text-slate-500">No high risk customers.</td></tr>
                      )}
                    </tbody>
                  </table>
                </div>
              </div>

              <div className="bg-slate-900/50 border border-slate-800 rounded-2xl overflow-hidden">
                <div className="px-5 py-4 border-b border-slate-800 font-semibold text-sm">Abuse Event Stream</div>
                <div className="divide-y divide-slate-800 max-h-[460px] overflow-auto">
                  {(adminAbuse.events || []).map(event => (
                    <div key={event.id} className="px-5 py-4">
                      <div className="flex items-center justify-between gap-3">
                        <span className="text-sm text-slate-200">{event.message}</span>
                        <span className={`px-2 py-0.5 rounded-full border text-[10px] uppercase ${getStatusColor(event.severity)}`}>{event.severity}</span>
                      </div>
                      <div className="mt-1 text-xs text-slate-500 font-mono">
                        {event.customer_id || 'anonymous'} / {event.proxy_id || 'n/a'} / {event.rule_id || 'policy'} / {event.created_at ? new Date(event.created_at).toLocaleString() : ''}
                      </div>
                    </div>
                  ))}
                  {(adminAbuse.events || []).length === 0 && (
                    <div className="px-5 py-10 text-center text-sm text-slate-500">No abuse events recorded.</div>
                  )}
                </div>
              </div>
            </div>
          )}

          {/* Distributed Agents Panel */}
          {activeTab === 'agents' && (
            <div className="flex flex-col gap-6">
              {agentTokenNotice && (
                <div className="bg-emerald-950/30 border border-emerald-500/20 rounded-2xl p-4 flex flex-col gap-2">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <h3 className="text-sm font-semibold text-emerald-300 flex items-center gap-2">
                        <Key className="h-4 w-4" /> Agent token visible once
                      </h3>
                      <p className="text-xs text-emerald-100/70 mt-1">Agent ID: <span className="font-mono">{agentTokenNotice.agentId}</span></p>
                    </div>
                    <button
                      onClick={() => setAgentTokenNotice(null)}
                      className="text-xs px-3 py-1.5 rounded-lg border border-emerald-500/20 text-emerald-200 hover:bg-emerald-500/10"
                    >
                      Dismiss
                    </button>
                  </div>
                  <code className="block bg-slate-950 border border-emerald-500/10 rounded-xl px-3 py-2 text-xs text-emerald-200 break-all">
                    {agentTokenNotice.token}
                  </code>
                </div>
              )}

              {/* Agent Registry Form */}
              <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5 backdrop-blur-md">
                <h3 className="font-semibold text-sm mb-4 flex items-center gap-2">
                  <Server className="h-4 w-4 text-violet-400" /> Register Distributed Agent Node
                </h3>
                <form onSubmit={handleRegisterAgent} className="grid grid-cols-1 md:grid-cols-4 gap-4 items-end">
                  <div>
                    <label className="text-[10px] text-slate-400 block mb-1">Agent Name</label>
                    <input 
                      type="text" 
                      value={newAgentName}
                      onChange={(e) => setNewAgentName(e.target.value)}
                      placeholder="Agent-Berlin-01" 
                      className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-1.5 text-xs text-slate-100 placeholder-slate-650 focus:outline-none"
                      required
                    />
                  </div>
                  <div>
                    <label className="text-[10px] text-slate-400 block mb-1">Hostname / Domain</label>
                    <input 
                      type="text" 
                      value={newAgentHost}
                      onChange={(e) => setNewAgentHost(e.target.value)}
                      placeholder="berlin.agent.internal" 
                      className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-1.5 text-xs text-slate-100 placeholder-slate-650 focus:outline-none"
                      required
                    />
                  </div>
                  <div>
                    <label className="text-[10px] text-slate-400 block mb-1">IP Address</label>
                    <input 
                      type="text" 
                      value={newAgentIP}
                      onChange={(e) => setNewAgentIP(e.target.value)}
                      placeholder="185.25.12.33" 
                      className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-1.5 text-xs text-slate-100 placeholder-slate-650 focus:outline-none"
                      required
                    />
                  </div>
                  <div className="flex gap-2">
                    <div className="flex-1">
                      <label className="text-[10px] text-slate-400 block mb-1">OS Type</label>
                      <select 
                        value={newAgentOS}
                        onChange={(e) => setNewAgentOS(e.target.value)}
                        className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-1.5 text-xs text-slate-100 focus:outline-none"
                      >
                        <option value="linux">Linux</option>
                        <option value="windows">Windows</option>
                        <option value="macos">macOS</option>
                      </select>
                    </div>
                    <button 
                      type="submit"
                      disabled={isSubmittingAgent}
                      className="bg-violet-600 hover:bg-violet-500 disabled:opacity-50 text-white rounded-xl px-4 py-1.5 text-xs font-semibold h-fit flex items-center justify-center gap-1.5"
                    >
                      <Plus className="h-3.5 w-3.5" /> Save
                    </button>
                  </div>
                </form>
              </div>

              {/* Agent Grid */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {agents.map(agent => (
                  <div key={agent.id} className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5 flex flex-col justify-between gap-4 backdrop-blur-md hover:border-slate-700 transition-all duration-300">
                    <div>
                      <div className="flex items-start justify-between">
                        <div className="flex items-center gap-2.5">
                          <div className="p-2 rounded-xl bg-slate-950 border border-slate-850 text-violet-400">
                            <Server className="h-4 w-4" />
                          </div>
                          <div>
                            <h4 className="font-bold text-sm text-slate-200">{agent.name}</h4>
                            <span className="text-[10px] text-slate-500 font-mono">{agent.hostname} ({agent.ip_address})</span>
                          </div>
                        </div>
                        <span className={`px-2 py-0.5 text-[10px] font-semibold rounded-full border uppercase tracking-wider ${getStatusColor(agent.status)}`}>
                          {agent.status}
                        </span>
                      </div>

                      {/* Agent Specs & Metrics */}
                      <div className="mt-4 grid grid-cols-2 gap-3 pt-3 border-t border-slate-850">
                        <div className="bg-slate-950/60 rounded-xl p-2.5 border border-slate-850/50 text-center">
                          <span className="text-[9px] text-slate-500 block uppercase font-medium">VPN Sessions</span>
                          <span className="text-sm font-bold text-slate-300 mt-0.5 block">{agent.vpn_count || 0} active</span>
                        </div>
                        <div className="bg-slate-950/60 rounded-xl p-2.5 border border-slate-850/50 text-center">
                          <span className="text-[9px] text-slate-500 block uppercase font-medium">Proxy Ports</span>
                          <span className="text-sm font-bold text-slate-300 mt-0.5 block">{agent.proxy_count || 0} mapped</span>
                        </div>
                        <div className="bg-slate-950/60 rounded-xl p-2.5 border border-slate-850/50 text-center">
                          <span className="text-[9px] text-slate-500 block uppercase font-medium">CPU</span>
                          <span className="text-sm font-bold text-slate-300 mt-0.5 block">{Math.round(agent.cpu_usage || 0)}%</span>
                        </div>
                        <div className="bg-slate-950/60 rounded-xl p-2.5 border border-slate-850/50 text-center">
                          <span className="text-[9px] text-slate-500 block uppercase font-medium">RAM</span>
                          <span className="text-sm font-bold text-slate-300 mt-0.5 block">{Math.round(agent.ram_usage || 0)}%</span>
                        </div>
                      </div>

                      <div className="mt-3 text-[10px] text-slate-500 flex justify-between items-center font-mono">
                        <span>Last Heartbeat:</span>
                        <span>{agent.last_heartbeat_at ? new Date(agent.last_heartbeat_at).toLocaleTimeString() : 'Never'}</span>
                      </div>
                    </div>

                    <div className="flex justify-end gap-2 pt-2 border-t border-slate-800/40">
                      {agent.id !== 'local-agent' && (
                        <>
                          <button 
                            onClick={() => handleRotateAgentToken(agent.id)}
                            className="px-2.5 py-1 text-indigo-300 hover:bg-indigo-500/10 border border-indigo-500/10 rounded-lg text-xs font-semibold flex items-center gap-1 transition-all"
                          >
                            <RefreshCw className="h-3 w-3" /> Rotate Token
                          </button>
                          <button 
                            onClick={() => handleRevokeAgent(agent.id)}
                            className="px-2.5 py-1 text-amber-300 hover:bg-amber-500/10 border border-amber-500/10 rounded-lg text-xs font-semibold flex items-center gap-1 transition-all"
                          >
                            <Power className="h-3 w-3" /> Revoke
                          </button>
                          <button 
                            onClick={() => handleDeleteAgent(agent.id)}
                            className="px-2.5 py-1 text-rose-400 hover:bg-rose-500/10 border border-rose-500/10 rounded-lg text-xs font-semibold flex items-center gap-1 transition-all"
                          >
                            <Trash2 className="h-3 w-3" /> Remove
                          </button>
                        </>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {activeTab === 'commands' && (
            <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5 flex flex-col gap-4 backdrop-blur-md">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Database className="h-4 w-4 text-indigo-400" />
                  <h2 className="font-semibold text-sm">Command Lifecycle Audit</h2>
                </div>
                <button
                  onClick={() => refetchCommands()}
                  className="p-2 rounded-lg border border-slate-800 text-slate-400 hover:text-slate-100 hover:bg-slate-800"
                  title="Refresh commands"
                >
                  <RefreshCw className="h-4 w-4" />
                </button>
              </div>
              <div className="grid grid-cols-[1.2fr_1fr_1fr_0.8fr_0.8fr_1.2fr] gap-3 px-3 text-[10px] uppercase tracking-wider text-slate-500 font-semibold">
                <span>Command ID</span>
                <span>Agent</span>
                <span>Type</span>
                <span>Status</span>
                <span>Attempts</span>
                <span>Timeline / Result</span>
              </div>
              <div className="bg-slate-950 border border-slate-850 rounded-xl h-[420px] overflow-auto">
                {filteredCommands.length === 0 ? (
                  <div className="text-slate-600 text-center py-20 text-xs">No command entries found.</div>
                ) : (
                  filteredCommands.map((cmd, index) => (
                    <CommandRow key={cmd.id || index} index={index} style={{}} />
                  ))
                )}
              </div>
            </div>
          )}

          {/* System Health Tab */}
          {activeTab === 'health' && (
            <div className="flex flex-col gap-6">
              {/* Health Overview Metric cards */}
              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5 flex flex-col items-center justify-center text-center backdrop-blur-md">
                  <span className="text-xs text-slate-500 font-semibold uppercase tracking-wider">Overall Grade</span>
                  <span className={`text-4xl font-extrabold tracking-tighter mt-2 px-6 py-2 rounded-full border shadow-lg ${
                    stats.health_grade === 'A' ? 'text-emerald-400 border-emerald-500/20 bg-emerald-500/5 shadow-emerald-500/10' :
                    stats.health_grade === 'B' ? 'text-amber-400 border-amber-500/20 bg-amber-500/5 shadow-amber-500/10' :
                    'text-rose-400 border-rose-500/20 bg-rose-500/5 shadow-rose-500/10'
                  }`}>
                    {stats.health_grade || 'A'}
                  </span>
                  <span className="text-[10px] text-slate-500 mt-3">Calculated from agent status & latency</span>
                </div>

                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5 flex flex-col items-center justify-center text-center backdrop-blur-md">
                  <span className="text-xs text-slate-500 font-semibold uppercase tracking-wider">Direct Latency</span>
                  <span className="text-3xl font-extrabold text-slate-200 tracking-tight mt-3">
                    {stats.health_latency || 0} <span className="text-sm font-semibold text-slate-500">ms</span>
                  </span>
                  <span className="text-[10px] text-slate-500 mt-4">Average response speed to exit node</span>
                </div>

                <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5 flex flex-col items-center justify-center text-center backdrop-blur-md">
                  <span className="text-xs text-slate-500 font-semibold uppercase tracking-wider">Validation status</span>
                  <span className="text-3xl font-extrabold text-indigo-400 tracking-tight mt-3">
                    PASS
                  </span>
                  <span className="text-[10px] text-slate-500 mt-4">ExpressVPN sanity validation checks</span>
                </div>
              </div>

              {/* ExpressVPN Node Sanity Checker Validator */}
              <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-6 backdrop-blur-md">
                <div className="flex items-center justify-between mb-4">
                  <div className="flex items-center gap-2">
                    <Wifi className="h-4.5 w-4.5 text-violet-400" />
                    <h3 className="font-bold text-sm text-slate-200">ExpressVPN Node Sanity Validator</h3>
                  </div>
                  <span className="text-xs text-slate-500">Diagnostic utility</span>
                </div>

                <div className="flex gap-3 items-center mb-6">
                  <div className="flex-1">
                    <select
                      value={selectedValNode}
                      onChange={(e) => setSelectedValNode(e.target.value)}
                      className="w-full bg-slate-950 border border-slate-800 rounded-xl px-3 py-2 text-xs text-slate-200"
                    >
                      <option value="">-- Choose Node to Validate --</option>
                      {vpns.filter(v => v.provider === 'expressvpn' || v.provider === 'ExpressVPN' || v.type === 'expressvpn' || v.type === 'expressvpn_mock').map(v => (
                        <option key={v.id} value={v.id}>{v.name} ({v.locationDisplayName || v.locationAlias})</option>
                      ))}
                    </select>
                  </div>
                  <button
                    type="button"
                    onClick={handleValidateNode}
                    disabled={isValidating || !selectedValNode}
                    className="bg-violet-650 hover:bg-violet-550 disabled:opacity-50 text-white rounded-xl px-4 py-2 text-xs font-semibold flex items-center gap-2 transition-all"
                  >
                    {isValidating ? (
                      <>
                        <Loader2 className="w-3.5 h-3.5 animate-spin" /> Checking...
                      </>
                    ) : (
                      'Trigger Sanity Validation'
                    )}
                  </button>
                </div>

                {/* Validation Report Details */}
                {validationReport && (
                  <div className="border border-slate-800 bg-slate-950/40 rounded-xl p-4 space-y-4 text-xs animate-fadeIn">
                    <div className="flex justify-between items-center pb-2.5 border-b border-slate-800/60">
                      <span className="font-semibold text-slate-300">Sanity Report Checklist</span>
                      <span className={`px-2 py-0.5 rounded text-[10px] font-bold uppercase ${
                        validationReport.status === 'success' ? 'bg-emerald-600/10 text-emerald-400 border border-emerald-500/20' : 'bg-rose-600/10 text-rose-400 border border-rose-500/20'
                      }`}>
                        {validationReport.status}
                      </span>
                    </div>

                    {validationReport.report && (
                      <div className="grid grid-cols-2 gap-4 text-[11px]">
                        <div className="space-y-2">
                          <div className="flex justify-between">
                            <span className="text-slate-500">CLI Binary Path:</span>
                            <span className={validationReport.report.cli_available ? 'text-emerald-400 font-medium' : 'text-rose-400 font-medium'}>
                              {validationReport.report.cli_available ? 'Found' : 'Not Found'}
                            </span>
                          </div>
                          <div className="flex justify-between">
                            <span className="text-slate-500">CLI Activated:</span>
                            <span className={validationReport.report.activated ? 'text-emerald-400 font-medium' : 'text-rose-400 font-medium'}>
                              {validationReport.report.activated ? 'Yes' : 'No'}
                            </span>
                          </div>
                          <div className="flex justify-between">
                            <span className="text-slate-500">Connect Test:</span>
                            <span className={validationReport.report.connect_success ? 'text-emerald-400 font-medium' : 'text-rose-400 font-medium'}>
                              {validationReport.report.connect_success ? 'Pass' : 'Fail'}
                            </span>
                          </div>
                          <div className="flex justify-between">
                            <span className="text-slate-500">Disconnect Test:</span>
                            <span className={validationReport.report.disconnect_success ? 'text-emerald-400 font-medium' : 'text-rose-400 font-medium'}>
                              {validationReport.report.disconnect_success ? 'Pass' : 'Fail'}
                            </span>
                          </div>
                        </div>

                        <div className="space-y-2 border-l border-slate-850 pl-4">
                          <div className="flex justify-between">
                            <span className="text-slate-500">Original IP:</span>
                            <span className="font-mono text-slate-300">{validationReport.report.original_ip}</span>
                          </div>
                          <div className="flex justify-between">
                            <span className="text-slate-500">Connected IP:</span>
                            <span className="font-mono text-emerald-400 font-medium">{validationReport.report.connected_ip}</span>
                          </div>
                          <div className="flex justify-between">
                            <span className="text-slate-500">IP Verification Change:</span>
                            <span className={validationReport.report.ip_changed ? 'text-emerald-400 font-medium' : 'text-rose-400 font-medium'}>
                              {validationReport.report.ip_changed ? 'Changed' : 'No Change'}
                            </span>
                          </div>
                          <div className="flex justify-between">
                            <span className="text-slate-500">Proxy SOCKS5 Routing:</span>
                            <span className="text-emerald-400 font-medium">Active</span>
                          </div>
                        </div>
                      </div>
                    )}

                    <div className="pt-2 border-t border-slate-800/40">
                      <span className="text-[10px] text-slate-500 block mb-1">Raw Output Logs:</span>
                      <pre className="bg-slate-950 p-2.5 rounded-lg border border-slate-850 font-mono text-[9px] text-slate-400 overflow-x-auto max-h-[140px] whitespace-pre-wrap leading-relaxed">
                        {validationReport.details}
                      </pre>
                    </div>
                  </div>
                )}
              </div>
            </div>
          )}
          
          {/* Real-time System Audit Logs */}
          <div className="bg-slate-900/50 border border-slate-800 rounded-2xl p-5 flex flex-col gap-4 backdrop-blur-md">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Terminal className="h-4 w-4 text-indigo-400" />
                <h2 className="font-semibold text-sm">System Audit Console</h2>
              </div>
              <input 
                type="text" 
                value={logFilter}
                onChange={(e) => setLogFilter(e.target.value)}
                placeholder="Filter action or detail..."
                className="bg-slate-950 border border-slate-800 rounded-xl px-3 py-1 text-xs text-slate-300 focus:outline-none focus:border-violet-600 max-w-[200px]"
              />
            </div>

            <div className="bg-slate-950 border border-slate-850 rounded-xl p-2 h-48 overflow-auto">
              {filteredLogs.length === 0 ? (
                <div className="text-slate-600 text-center py-10 text-xs">No log entries found.</div>
              ) : (
                filteredLogs.map((logItem, index) => (
                  <LogRow key={`${logItem.timestamp || index}-${logItem.action || 'log'}`} index={index} style={{}} />
                ))
              )}
            </div>
          </div>
        </div>
      </main>
    </div>
  );
}
